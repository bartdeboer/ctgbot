package claude

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
)

const (
	Type                 = "claude"
	DefaultDockerfile    = "claude.Dockerfile"
	stopAfterTurnTimeout = 5 * time.Second
)

var _ component.Agent = (*Component)(nil)
var _ component.ProfileOwner = (*Component)(nil)
var _ component.AuthStatusReporter = (*Component)(nil)
var _ component.RuntimeImageProvider = (*Component)(nil)

type TurnRunner interface {
	RunTurn(ctx context.Context, runtime ExecRuntime, request TurnRequest) (TurnResult, error)
}

type Component struct {
	registration      coremodel.Component
	runtime           runtimepkg.Runtime
	storage           repository.Storage
	resolveWorkspace  func(context.Context, coremodel.Chat) (string, error)
	componentConfig   ComponentConfig
	runner            TurnRunner
	logger            *log.Logger
	runtimeImage      string
	runtimeDockerfile string
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
	runtime := runtimeFactory.Bind(registration, home, bindConfig)
	return &Component{
		registration:      registration,
		runtime:           runtime,
		storage:           storage,
		resolveWorkspace:  resolveWorkspace,
		componentConfig:   componentConfig,
		runner:            NewRunner(logger),
		logger:            logger,
		runtimeImage:      bindConfig.Image,
		runtimeDockerfile: firstNonEmpty(componentConfig.Dockerfile, DefaultDockerfile),
	}, nil
}

func (c *Component) Type() string { return Type }

func (c *Component) RuntimeImageTargets(ctx context.Context) ([]runtimeimage.Target, error) {
	_ = ctx
	if c == nil || (c.runtime != nil && c.runtime.Kind() != "docker") {
		return nil, nil
	}
	return []runtimeimage.Target{{
		Name:       Type,
		Ref:        c.registration.Ref(),
		Image:      firstNonEmpty(c.runtimeImage, DefaultImage),
		Dockerfile: firstNonEmpty(c.runtimeDockerfile, DefaultDockerfile),
	}}, nil
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

func (c *Component) AuthStatus(ctx context.Context, stdout io.Writer, stderr io.Writer) error {
	if c == nil || c.runtime == nil {
		return fmt.Errorf("missing component runtime")
	}
	if err := PrepareHome(HomeSpec{HostHome: c.runtime.ComponentHome().Path}); err != nil {
		return err
	}
	return c.runtime.Exec(ctx, "", modeluuid.UUID{}, nil, writerOrDiscard(stdout), writerOrDiscard(stderr), "claude", "--version")
}

func (c *Component) HandleTurn(ctx context.Context, turn component.Turn) (*component.TurnResult, error) {
	if c == nil || c.runner == nil {
		return nil, fmt.Errorf("missing claude runner")
	}
	if c.runtime == nil {
		return nil, fmt.Errorf("missing component runtime")
	}
	prompt := strings.TrimSpace(turn.Inbound.Text)
	if prompt == "" {
		return nil, nil
	}
	workspacePath := turn.Runtime.WorkspacePath()
	runtimeWorkspacePath := c.runtime.RuntimeWorkspacePath(workspacePath)
	instructions := turn.Runtime.Instructions()
	instructions.RuntimeNotices = append(instructions.RuntimeNotices, c.runtimeNotices(ctx, workspacePath, turn.Thread.ID)...)
	bootstrapText := claudeBootstrap(runtimeWorkspacePath, instructions)
	if err := PrepareHome(HomeSpec{HostHome: c.runtime.ComponentHome().Path, BootstrapText: bootstrapText}); err != nil {
		return nil, err
	}
	stopTyping, err := turn.Runtime.StartChatAction(ctx, message.ChatActionTyping)
	if err == nil && stopTyping != nil {
		defer stopTyping()
	}
	providerThreadID, err := c.providerThreadID(turn.Runtime)
	if err != nil {
		return nil, err
	}
	settings, err := c.resolveThreadSettings(ctx, &turn.Thread)
	if err != nil {
		return nil, err
	}
	result, runErr := c.runner.RunTurn(ctx, commandRuntime{runtime: c.runtime, threadID: turn.Thread.ID, workspacePath: workspacePath, commands: turn.Runtime.Commands()}, TurnRequest{
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
		c.stopAfterTurn(workspacePath, turn.Thread.ID)
	}
	if saveErr := c.bindComponentThreadID(turn.Runtime, result.ProviderThreadID); saveErr != nil && runErr == nil {
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
	return &component.TurnResult{Final: &coremodel.ThreadMessage{Kind: coremodel.MessageKindAgent, ComponentID: c.registration.ID, ActorID: c.registration.Ref(), ActorLabel: "Claude", Text: reply}}, nil
}

func (c *Component) providerThreadID(turnRuntime component.TurnRuntime) (string, error) {
	componentThreadID, ok, err := turnRuntime.ComponentThreadID(c.registration.ID)
	if err != nil || !ok {
		return "", err
	}
	return strings.TrimSpace(componentThreadID), nil
}

func (c *Component) runtimeNotices(ctx context.Context, workspacePath string, threadID modeluuid.UUID) []string {
	if c == nil || c.runtime == nil {
		return nil
	}
	status, err := c.runtime.Status(ctx, workspacePath, threadID)
	if err != nil {
		c.logf("runtime notice status check failed thread=%s err=%v", threadID, err)
		return nil
	}
	return append([]string(nil), status.RuntimeNotices...)
}

func (c *Component) bindComponentThreadID(turnRuntime component.TurnRuntime, providerThreadID string) error {
	providerThreadID = strings.TrimSpace(providerThreadID)
	if providerThreadID == "" {
		return nil
	}
	return turnRuntime.BindComponentThreadID(c.registration.ID, providerThreadID)
}

func (c *Component) stopAfterTurn(workspacePath string, threadID modeluuid.UUID) {
	if c == nil || c.runtime == nil {
		return
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), stopAfterTurnTimeout)
	defer cancel()
	if err := c.runtime.Stop(stopCtx, workspacePath, threadID); err != nil {
		c.logf("claude stop-after-turn failed thread=%s err=%v", threadID, err)
	}
}

func componentBindConfig(config runtimepkg.BindConfig, componentConfig ComponentConfig, runtimeHomePath string) runtimepkg.BindConfig {
	config = config.Clean()
	config.Image = firstNonEmpty(componentConfig.Image, config.Image, DefaultImage)
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
	allowedCommandsText := strings.Join(instructions.HostbridgeCommandNames, ", ")
	if strings.TrimSpace(allowedCommandsText) == "" {
		allowedCommandsText = "<none>"
	}
	lines := []string{
		"You are Claude Code running inside ctgbot.",
		"Current workspace: " + workspace,
		"Workspace inbox: " + workspace + "/inbox",
		"Container OS: linux",
		"Host OS: " + goruntime.GOOS,
		"Chat provider: " + chatProvider,
	}
	if prefix := strings.TrimSpace(instructions.MessagePrefix); prefix != "" {
		lines = append(lines, "Start every final assistant message with `"+prefix+"`.")
	}
	if instructions.KeepRepliesConcise {
		lines = append(lines, "Keep replies concise.")
	}
	lines = append(lines, "Available hostbridge command aliases: "+allowedCommandsText)
	if len(instructions.HostbridgeControlCommands) > 0 {
		lines = append(lines, "Useful hostbridge control commands:")
		for _, command := range instructions.HostbridgeControlCommands {
			lines = append(lines, "- `"+command+"`")
		}
	}
	for _, notice := range instructions.RuntimeNotices {
		if notice = strings.TrimSpace(notice); notice != "" {
			lines = append(lines, notice)
		}
	}
	return strings.Join(lines, "\n")
}

func writerOrDiscard(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}

func (c *Component) logf(format string, args ...any) {
	if c != nil && c.logger != nil {
		c.logger.Printf(format, args...)
	}
}

type commandRuntime struct {
	runtime       runtimepkg.Runtime
	threadID      modeluuid.UUID
	workspacePath string
	commands      commandengine.CommandExecutor
}

func (r commandRuntime) Workspace() string { return r.runtime.RuntimeWorkspacePath(r.workspacePath) }

func (r commandRuntime) Exec(ctx context.Context, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	return r.runtime.Exec(ctx, r.workspacePath, r.threadID, r.commands, stdout, stderr, name, args...)
}
