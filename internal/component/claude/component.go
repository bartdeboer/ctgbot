package claude

import (
	"context"
	"errors"
	"fmt"
	"io"
	goruntime "runtime"
	"strings"
	"time"

	"log"

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
	Type                 = "claude"
	DefaultCallbackPort  = 1455
	DefaultBaseImage     = "ctgbot-claude-base:latest"
	DefaultDevBaseImage  = "ctgbot-go-node-python-base:latest"
	DefaultDockerfile    = "claude.Dockerfile"
	stopAfterTurnTimeout = agentcommon.DefaultStopAfterTurnTimeout
)

var _ component.Agent = (*Component)(nil)
var _ component.ProfileOwner = (*Component)(nil)
var _ component.Authenticator = (*Component)(nil)
var _ component.AuthStatusReporter = (*Component)(nil)
var _ component.RuntimeImageProvider = (*Component)(nil)
var _ component.ThreadRuntimeController = (*Component)(nil)

type turnRunner interface {
	RunTurn(ctx context.Context, runtime ExecRuntime, request TurnRequest) (TurnResult, error)
}

type Component struct {
	agentcommon.Core
	componentConfig ComponentConfig
	runner          turnRunner
}

func New(ctx context.Context, registration coremodel.Component, runtimeFactory runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage, resolveWorkspace func(context.Context, coremodel.Chat) (string, error), logger *log.Logger) (component.Component, error) {
	_ = ctx
	if runtimeFactory == nil {
		return nil, fmt.Errorf("missing runtime factory")
	}
	if storage == nil {
		return nil, fmt.Errorf("missing storage")
	}
	if resolveWorkspace == nil {
		return nil, fmt.Errorf("missing workspace resolver")
	}
	runtimeConfig, err := runtimepkg.LoadBindConfig(home.Path)
	if err != nil {
		return nil, err
	}
	componentConfig, err := loadComponentConfig(home.Path)
	if err != nil {
		return nil, err
	}
	runtimeHomePath := runtimeFactory.RuntimeComponentHomePath(registration, home)
	bindConfig := componentBindConfig(runtimeConfig, componentConfig, runtimeHomePath)
	threadFactory, ok := runtimeFactory.(runtimepkg.ThreadRuntimeFactory)
	if !ok {
		return nil, fmt.Errorf("claude requires thread runtime, got %T", runtimeFactory)
	}
	runtime := threadFactory.Bind(registration, home, bindConfig)
	return &Component{
		Core: agentcommon.Core{
			Registration:        registration,
			Runtime:             runtime,
			Storage:             storage,
			ResolveWorkspace:    resolveWorkspace,
			Logger:              logger,
			RuntimeImage:        bindConfig.Image,
			RuntimeDockerfile:   agentcommon.FirstNonEmpty(bindConfig.Dockerfile, componentConfig.Dockerfile, DefaultDockerfile),
			RuntimeImageUses:    bindConfig.Uses,
			RuntimeImageNoCache: bindConfig.NoCache,
		},
		componentConfig: componentConfig,
		runner:          NewRunner(logger),
	}, nil
}

func (c *Component) Type() string { return Type }

func (c *Component) RuntimeImageTargets(ctx context.Context) ([]runtimeimage.Target, error) {
	_ = ctx
	if c == nil || (c.Runtime != nil && c.Runtime.Kind() != "docker") {
		return nil, nil
	}
	target := runtimeimage.Target{
		Name:       Type,
		Image:      agentcommon.FirstNonEmpty(c.RuntimeImage, DefaultImage),
		Dockerfile: agentcommon.FirstNonEmpty(c.RuntimeDockerfile, DefaultDockerfile),
		NoCache:    c.RuntimeImageNoCache,
		Uses:       c.RuntimeImageUses,
	}
	if target.Uses != nil {
		if !target.NoCache {
			target.NoCache = true
		}
		return []runtimeimage.Target{target}, nil
	}
	if target.Dockerfile != DefaultDockerfile {
		return []runtimeimage.Target{target}, nil
	}
	base := runtimeimage.Target{
		Name:       Type + "-base",
		Image:      DefaultBaseImage,
		Dockerfile: "claude.base.Dockerfile",
		Uses: &runtimeimage.Target{
			Name:       "go-node-python-base",
			Image:      DefaultDevBaseImage,
			Dockerfile: "go-node-python.base.Dockerfile",
		},
	}
	target.Uses = &base
	target.NoCache = true
	return []runtimeimage.Target{target}, nil
}

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{
		{RelativePath: ComponentConfigFilename, Required: false, Sensitive: false},
		{RelativePath: runtimepkg.ConfigFilename, Required: false, Sensitive: false},
		{RelativePath: "ctgbot-bootstrap.md", Required: false, Sensitive: false},
		{RelativePath: ".claude/settings.json", Required: false, Sensitive: false},
		{RelativePath: ".claude.json", Required: false, Sensitive: true},
	}
}

func (c *Component) Auth(ctx context.Context, callbackPort int, callbackTimeout time.Duration, stdout io.Writer, stderr io.Writer) error {
	if c == nil || c.Runtime == nil {
		return fmt.Errorf("missing component runtime")
	}
	if err := PrepareHome(HomeSpec{HostHome: c.Runtime.ComponentHome().Path}); err != nil {
		return err
	}
	closeRelay, err := c.Runtime.OpenHTTPRelayPort(
		ctx,
		"",
		modeluuid.UUID{},
		nil,
		claudeCallbackPort(callbackPort),
		claudeCallbackTimeout(callbackTimeout),
	)
	if err != nil {
		return err
	}
	defer func() { _ = closeRelay(context.Background()) }()
	return c.Runtime.ExecTTY(ctx, "", modeluuid.UUID{}, nil, agentcommon.WriterOrDiscard(stdout), agentcommon.WriterOrDiscard(stderr), "env", "BROWSER=echo", "claude", "setup-token")
}

func (c *Component) AuthStatus(ctx context.Context, stdout io.Writer, stderr io.Writer) error {
	if c == nil || c.Runtime == nil {
		return fmt.Errorf("missing component runtime")
	}
	if err := PrepareHome(HomeSpec{HostHome: c.Runtime.ComponentHome().Path}); err != nil {
		return err
	}
	return c.Runtime.Exec(ctx, "", modeluuid.UUID{}, nil, agentcommon.WriterOrDiscard(stdout), agentcommon.WriterOrDiscard(stderr), "claude", "--version")
}

func (c *Component) HandleTurn(ctx context.Context, turn component.Turn) (*component.TurnResult, error) {
	if c == nil || c.runner == nil {
		return nil, fmt.Errorf("missing claude runner")
	}
	if c.Runtime == nil {
		return nil, fmt.Errorf("missing component runtime")
	}
	prompt := strings.TrimSpace(turn.Inbound.Text)
	if prompt == "" {
		return nil, nil
	}
	workspacePath := turn.Runtime.WorkspacePath()
	runtimeWorkspacePath := c.Runtime.RuntimeWorkspacePath(workspacePath)
	instructions := turn.Runtime.Instructions()
	instructions.RuntimeNotices = append(instructions.RuntimeNotices, c.RuntimeNotices(ctx, workspacePath, turn.Thread.ID)...)
	bootstrapText := claudeBootstrap(runtimeWorkspacePath, instructions)
	if err := PrepareHome(HomeSpec{HostHome: c.Runtime.ComponentHome().Path, BootstrapText: bootstrapText}); err != nil {
		return nil, err
	}
	stopTyping, err := turn.Runtime.StartChatAction(ctx, message.ChatActionTyping)
	if err == nil && stopTyping != nil {
		defer stopTyping()
	}
	providerThreadID, err := c.ProviderThreadID(turn.Runtime)
	if err != nil {
		return nil, err
	}
	settings, err := c.resolveThreadSettings(ctx, &turn.Thread)
	if err != nil {
		return nil, err
	}
	result, runErr := c.runner.RunTurn(ctx, commandRuntime{runtime: c.Runtime, threadID: turn.Thread.ID, workspacePath: workspacePath, commands: turn.Runtime.Commands()}, TurnRequest{
		ProviderThreadID: providerThreadID,
		Prompt:           prompt,
		Options: TurnOptions{
			Model:             modelOption(settings),
			PermissionMode:    settings.PermissionMode,
			SystemPrompt:      bootstrapText,
			SessionTimeoutSec: settings.SessionTimeoutSec,
		},
	})
	if !settings.KeepRunning {
		c.StopAfterTurn(workspacePath, turn.Thread.ID, stopAfterTurnTimeout)
	}
	if saveErr := c.BindComponentThreadID(turn.Runtime, result.ProviderThreadID); saveErr != nil && runErr == nil {
		runErr = saveErr
	}
	if runErr != nil {
		if errors.Is(runErr, context.Canceled) && ctx.Err() == nil {
			return nil, nil
		}
		return nil, runErr
	}
	reply := strings.TrimSpace(result.Reply)
	if reply == "" {
		return nil, nil
	}
	return &component.TurnResult{Final: &coremodel.ThreadMessage{Role: coremodel.MessageRoleAgent, Kind: coremodel.MessageKindMessage, ComponentID: c.Registration.ID, ActorID: c.Registration.Ref(), ActorLabel: "Claude", Text: reply}}, nil
}

func componentBindConfig(config runtimepkg.BindConfig, componentConfig ComponentConfig, runtimeHomePath string) runtimepkg.BindConfig {
	config = config.Clean()
	config.Image = agentcommon.FirstNonEmpty(componentConfig.Image, config.Image, DefaultImage)
	return config.WithEnv("HOME="+runtimeHomePath, "CLAUDE_CONFIG_DIR="+runtimeHomePath+"/.claude")
}

func modelOption(settings resolvedThreadSettings) string {
	if settings.ModelSource == "claude" {
		return ""
	}
	return settings.Model
}

func claudeBootstrap(workspace string, instructions component.TurnInstructions) string {
	chatProvider := strings.TrimSpace(instructions.ChatProvider)
	if chatProvider == "" {
		chatProvider = "Chat"
	}
	runAliasSynopsis := agentcommon.CommandSynopsis("hostbridge run", instructions.HostbridgeCommandNames)
	controlSynopsis := ""
	if len(instructions.HostbridgeControlCommands) > 0 {
		controlSynopsis = agentcommon.HostbridgeSynopsis(instructions.HostbridgeControlCommands)
	}
	lines := []string{
		"You are Claude Code running inside ctgbot.",
		"",
		"Environment:",
		"- Current workspace: " + workspace,
		"- Workspace inbox: " + workspace + "/inbox",
		"- Container OS: linux",
		"- Host OS: " + goruntime.GOOS,
		"- Chat provider: " + chatProvider,
	}
	if prefix := strings.TrimSpace(instructions.MessagePrefix); prefix != "" {
		lines = append(lines, "Start every final assistant message with `"+prefix+"`.")
	}
	if instructions.KeepRepliesConcise {
		lines = append(lines, "Keep replies concise.")
	}
	lines = append(lines, "Do not add Co-Authored-By trailers to commits unless the operator explicitly asks for them.")
	lines = append(lines, "When messaging threads, end your turn to receive their response. Do not poll for replies.")
	lines = append(lines, "Use `hostbridge turn info` and `hostbridge turn config [ list | get <key> | set <key> <value> ]` for current-turn input metadata and output controls.")
	lines = append(lines, "Use `hostbridge model <name> card` for model config options.")
	if strings.TrimSpace(controlSynopsis) != "" {
		lines = append(lines, "", "Canonical hostbridge commands:", "```text", controlSynopsis, "```")
	}
	lines = append(lines, "", "Available hostbridge run aliases (on host):", "```text", runAliasSynopsis, "```")
	for _, notice := range instructions.RuntimeNotices {
		if notice = strings.TrimSpace(notice); notice != "" {
			lines = append(lines, notice)
		}
	}
	return strings.Join(lines, "\n")
}

func claudeCallbackPort(port int) int {
	if port <= 0 {
		return DefaultCallbackPort
	}
	return port
}

func claudeCallbackTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return 10 * time.Minute
	}
	return timeout
}

type commandRuntime struct {
	runtime       runtimepkg.ThreadRuntime
	threadID      modeluuid.UUID
	workspacePath string
	commands      commandengine.CommandExecutor
}

func (r commandRuntime) Exec(ctx context.Context, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	return r.runtime.Exec(ctx, r.workspacePath, r.threadID, r.commands, stdout, stderr, name, args...)
}
