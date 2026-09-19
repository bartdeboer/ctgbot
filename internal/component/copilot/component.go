package copilot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
)

const (
	Type                     = "copilot"
	DefaultImage             = "ctgbot-copilot:latest"
	DefaultDockerfile        = "copilot.Dockerfile"
	DefaultSessionTimeoutSec = 30 * 60
)

var (
	_ component.TurnHandler              = (*Component)(nil)
	_ component.ProfileOwner             = (*Component)(nil)
	_ component.Authenticator            = (*Component)(nil)
	_ component.AuthStatusReporter       = (*Component)(nil)
	_ component.RuntimeImageProvider     = (*Component)(nil)
	_ component.ThreadSandboxKeepRunning = (*Component)(nil)
)

type Component struct {
	agentcommon.Core
	config ComponentConfig
}

func New(_ context.Context, registration coremodel.Component, factory runtimepkg.Factory, profile runtimepkg.Profile, storage repository.Storage, resolveWorkspace func(context.Context, coremodel.Chat) (string, error), logger *log.Logger) (component.Component, error) {
	if factory == nil || factory.Kind() != "docker" {
		return nil, fmt.Errorf("copilot requires the Docker thread runtime")
	}
	threadFactory, ok := factory.(runtimepkg.ThreadRuntimeFactory)
	if !ok {
		return nil, fmt.Errorf("copilot requires a thread runtime factory")
	}
	if storage == nil || resolveWorkspace == nil {
		return nil, fmt.Errorf("missing copilot storage or workspace resolver")
	}
	bind, err := runtimepkg.LoadBindConfig(profile.Path)
	if err != nil {
		return nil, err
	}
	config, err := loadConfig(profile.Path)
	if err != nil {
		return nil, err
	}
	bind = bind.Clean()
	bind.Image = agentcommon.FirstNonEmpty(config.Image, bind.Image, DefaultImage)
	profilePath := factory.RuntimeComponentProfilePath(registration, profile)
	// Never reuse another tool's ambient token. Dedicated Copilot credentials may
	// be provisioned explicitly later; discovery neither opens nor creates them.
	env := make([]string, 0, len(bind.Env))
	for _, value := range bind.Env {
		key, _, _ := strings.Cut(value, "=")
		if key != "GH_TOKEN" && key != "GITHUB_TOKEN" {
			env = append(env, value)
		}
	}
	bind.Env = env
	bind = bind.WithEnv("HOME="+profilePath, "COPILOT_HOME="+profilePath+"/.copilot")
	return &Component{
		Core: agentcommon.Core{Registration: registration, Runtime: threadFactory.Bind(registration, profile, bind), Storage: storage, ResolveWorkspace: resolveWorkspace, Logger: logger,
			RuntimeImageContext: bind.Context, RuntimeImage: bind.Image, RuntimeDockerfile: agentcommon.FirstNonEmpty(bind.Dockerfile, config.Dockerfile, DefaultDockerfile), RuntimeImageUses: bind.Uses, RuntimeImageNoCache: bind.NoCache},
		config: config,
	}, nil
}

func (*Component) Type() string { return Type }

func (c *Component) HandleTurn(ctx context.Context, turn component.Turn) (*component.TurnResult, error) {
	if c == nil || c.Runtime == nil {
		return nil, fmt.Errorf("missing copilot runtime")
	}
	prompt := strings.TrimSpace(turn.PromptText())
	if prompt == "" {
		return nil, nil
	}
	if turn.Runtime == nil {
		return nil, fmt.Errorf("missing copilot turn runtime")
	}
	id, err := c.ProviderThreadID(turn.Runtime)
	if err != nil {
		return nil, err
	}
	if id != "" {
		if _, err := canonicalSessionID(id); err != nil {
			return nil, err
		}
	}
	settings, err := c.settings(ctx, &turn.Thread)
	if err != nil {
		return nil, err
	}
	workspace := turn.Runtime.WorkspacePath()
	instructions := turn.Runtime.Instructions()
	instructions.RuntimeNotices = append(instructions.RuntimeNotices, c.RuntimeNotices(ctx, workspace, turn.Thread.ID)...)
	path, cleanup, err := stagePrompt(c.Runtime.ComponentProfile().Path, c.Runtime.RuntimeComponentProfilePath(), bootstrap(c.Runtime.RuntimeWorkspacePath(workspace), instructions)+"\n\nUser request:\n"+prompt)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	if stop, err := turn.Runtime.StartChatAction(ctx, message.ChatActionTyping); err == nil && stop != nil {
		defer stop()
	}
	result, runErr := (Runner{}).RunTurn(ctx, commandRuntime{c.Runtime, workspace, turn.Thread.ID, turn.Runtime.Commands()}, TurnRequest{
		ProviderThreadID: id, PromptPath: path, UsagePath: filepath.Join(filepath.Dir(path), "usage.json"), Workspace: c.Runtime.RuntimeWorkspacePath(workspace), Model: settings.Model, SessionTimeoutSec: c.config.SessionTimeoutSec,
	})
	usage := readUsage(filepath.Join(c.Runtime.ComponentProfile().Path, filepath.Base(filepath.Dir(path)), "usage.json"))
	usage.ProviderSessionID = result.ProviderThreadID
	// Only Runner's validated expected UUID can reach this bind. Invalid/empty
	// output leaves an existing durable mapping untouched.
	bindErr := c.BindComponentThreadID(turn.Runtime, result.ProviderThreadID)
	// Read/validate/bind before teardown. Broker relays Final afterwards. This is
	// the existing best-effort direct-process policy, not proof of tree exit.
	if !settings.KeepRunning {
		c.StopAfterTurn(workspace, turn.Thread.ID, agentcommon.DefaultStopAfterTurnTimeout)
	}
	// Match existing agents: a runtime-reported operator interrupt is quiet;
	// caller cancellation/deadline failures still propagate.
	if bindErr == nil && errors.Is(runErr, context.Canceled) && ctx.Err() == nil {
		return nil, nil
	}
	if err := errors.Join(runErr, bindErr); err != nil {
		return nil, err
	}
	return &component.TurnResult{Final: &coremodel.ThreadMessage{
		Role: coremodel.MessageRoleAgent, Kind: coremodel.MessageKindMessage, ComponentID: c.Registration.ID,
		ActorID: c.Registration.Ref(), ActorLabel: "Copilot", Text: result.Reply, Usage: usage,
	}}, nil
}

func (*Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{
		{RelativePath: "component.json"}, {RelativePath: runtimepkg.ConfigFilename},
		{RelativePath: ".copilot/config.json", Sensitive: true},
	}
}

func (c *Component) Auth(ctx context.Context, _ int, timeout time.Duration, stdout, stderr io.Writer) error {
	if c == nil || c.Runtime == nil {
		return fmt.Errorf("missing copilot runtime")
	}
	if err := prepareProfile(c.Runtime.ComponentProfile().Path); err != nil {
		return err
	}
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// Explicit operator-only flow. Plaintext fallback consent remains interactive;
	// no callback port, automatic consent, secret scraping or inference probe.
	return c.Runtime.ExecTTY(ctx, "", modeluuid.UUID{}, nil, agentcommon.WriterOrDiscard(stdout), agentcommon.WriterOrDiscard(stderr), "env", "-u", "GH_TOKEN", "-u", "GITHUB_TOKEN", "copilot", "--no-auto-update", "login", "--device-code")
}

func (*Component) AuthStatus(_ context.Context, stdout, _ io.Writer) error {
	_, err := fmt.Fprintln(agentcommon.WriterOrDiscard(stdout), "copilot authentication: unverified (no credential or network probe); use explicit component auth, then verify account and employer CLI/model policy")
	return err
}

func (c *Component) RuntimeImageTargets(context.Context) ([]runtimeimage.Target, error) {
	if c == nil {
		return nil, nil
	}
	target := runtimeimage.Target{Context: c.RuntimeImageContext, Name: Type, Image: agentcommon.FirstNonEmpty(c.RuntimeImage, DefaultImage), Dockerfile: agentcommon.FirstNonEmpty(c.RuntimeDockerfile, DefaultDockerfile), Uses: c.RuntimeImageUses, NoCache: c.RuntimeImageNoCache}
	if target.Uses == nil && target.Context == "" && target.Dockerfile == DefaultDockerfile {
		target.Uses = &runtimeimage.Target{Name: "copilot-base", Image: "ctgbot-copilot-base:latest", Dockerfile: "copilot.base.Dockerfile", Uses: &runtimeimage.Target{Name: "go-node-python-base", Image: "ctgbot-go-node-python-base:latest", Dockerfile: "go-node-python.base.Dockerfile"}}
		target.NoCache = true
	}
	return []runtimeimage.Target{target}, nil
}

type commandRuntime struct {
	runtime   runtimepkg.ThreadRuntime
	workspace string
	thread    modeluuid.UUID
	commands  commandengine.CommandExecutor
}

func (r commandRuntime) Exec(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error {
	return r.runtime.Exec(ctx, r.workspace, r.thread, r.commands, stdout, stderr, name, args...)
}
