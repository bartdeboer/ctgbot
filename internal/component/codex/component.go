package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
	codexbootstrap "github.com/bartdeboer/ctgbot/internal/component/codex/bootstrap"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
)

const (
	// Type matches the existing Codex component registration name so callers can
	// switch implementations by changing only the import path.
	Type                 = "codex"
	DefaultImage         = "ctgbot-codex:latest"
	DefaultBaseImage     = "ctgbot-codex-base:latest"
	DefaultCallbackPort  = 1455
	stopAfterTurnTimeout = 5 * time.Second
)

var _ component.Agent = (*Component)(nil)
var _ component.RuntimeImageProvider = (*Component)(nil)

type turnRunner interface {
	RunTurn(ctx context.Context, runtime ExecRuntime, output OutputHandler, request TurnRequest) (TurnResult, error)
}

type Component struct {
	registration        coremodel.Component
	runtime             runtimepkg.ThreadRuntime
	storage             repository.Storage
	resolveWorkspace    func(context.Context, coremodel.Chat) (string, error)
	config              *appstate.Config
	componentConfig     ComponentConfig
	runner              turnRunner
	logger              *log.Logger
	runtimeImage        string
	runtimeDockerfile   string
	runtimeImageUses    *runtimeimage.Target
	runtimeImageNoCache bool
}

func New(
	ctx context.Context,
	registration coremodel.Component,
	runtimeFactory runtimepkg.Factory,
	home runtimepkg.Home,
	storage repository.Storage,
	cfg *appstate.Config,
	resolveWorkspace func(context.Context, coremodel.Chat) (string, error),
	logger *log.Logger,
	image string,
) (component.Component, error) {
	_ = ctx
	if cfg == nil {
		return nil, fmt.Errorf("missing config")
	}
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
	bindConfig := componentBindConfig(runtimeConfig, cfg, image, runtimeHomePath)
	threadFactory, ok := runtimeFactory.(runtimepkg.ThreadRuntimeFactory)
	if !ok {
		return nil, fmt.Errorf("codex requires thread runtime, got %T", runtimeFactory)
	}
	runtime := threadFactory.Bind(
		registration,
		home,
		bindConfig,
	)
	return &Component{
		registration:        registration,
		runtime:             runtime,
		storage:             storage,
		resolveWorkspace:    resolveWorkspace,
		config:              cfg,
		componentConfig:     componentConfig,
		runner:              NewRunner(cfg, logger),
		logger:              logger,
		runtimeImage:        bindConfig.Image,
		runtimeDockerfile:   firstNonEmpty(bindConfig.Dockerfile, cfg.Docker().Dockerfile()),
		runtimeImageUses:    bindConfig.Uses,
		runtimeImageNoCache: bindConfig.NoCache,
	}, nil
}

func (c *Component) Type() string {
	return Type
}

func (c *Component) RuntimeImageTargets(ctx context.Context) ([]runtimeimage.Target, error) {
	_ = ctx
	if c == nil {
		return nil, nil
	}
	if c.runtime != nil && c.runtime.Kind() != "docker" {
		return nil, nil
	}
	image := strings.TrimSpace(c.runtimeImage)
	if image == "" && c.config != nil {
		image = strings.TrimSpace(c.config.Docker().Image())
	}
	image = componentImage(image)
	dockerfile := strings.TrimSpace(c.runtimeDockerfile)
	if dockerfile == "" && c.config != nil {
		dockerfile = strings.TrimSpace(c.config.Docker().Dockerfile())
	}
	if dockerfile == "" {
		dockerfile = "codex.Dockerfile"
	}
	target := runtimeimage.Target{
		Name:       Type,
		Image:      image,
		Dockerfile: dockerfile,
		NoCache:    c.runtimeImageNoCache,
		Uses:       c.runtimeImageUses,
	}
	if target.Uses != nil {
		if !target.NoCache {
			target.NoCache = true
		}
		return []runtimeimage.Target{target}, nil
	}
	if dockerfile != "codex.Dockerfile" {
		return []runtimeimage.Target{target}, nil
	}
	base := runtimeimage.Target{
		Name:       Type + "-base",
		Image:      DefaultBaseImage,
		Dockerfile: "codex.base.Dockerfile",
	}
	target.Uses = &base
	target.NoCache = true
	return []runtimeimage.Target{target}, nil
}

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{
		{RelativePath: "auth.json", Required: true, Sensitive: true},
		{RelativePath: "config.toml", Required: false, Sensitive: false},
		{RelativePath: "ctgbot-bootstrap.md", Required: false, Sensitive: false},
		{RelativePath: runtimepkg.ConfigFilename, Required: false, Sensitive: false},
		{RelativePath: ComponentConfigFilename, Required: false, Sensitive: false},
	}
}

func (c *Component) Auth(ctx context.Context, callbackPort int, callbackTimeout time.Duration, stdout io.Writer, stderr io.Writer) error {
	if c == nil || c.runtime == nil {
		return fmt.Errorf("missing component runtime")
	}
	home := c.runtime.ComponentHome()
	runtimeHomePath := c.runtime.RuntimeComponentHomePath()
	if err := PrepareHome(c.config, HomeSpec{
		HostHome:         home.Path,
		RuntimeHome:      runtimeHomePath,
		RuntimeWorkspace: runtimeHomePath,
		SandboxMode:      c.componentConfig.SandboxMode,
	}); err != nil {
		return err
	}
	closeRelay, err := c.runtime.OpenHTTPRelayPort(
		ctx,
		"",
		modeluuid.UUID{},
		nil,
		callbackPortOrDefault(callbackPort),
		callbackTimeoutOrDefault(callbackTimeout),
	)
	if err != nil {
		return err
	}
	defer func() { _ = closeRelay(context.Background()) }()
	return c.runtime.Exec(
		ctx,
		"",
		modeluuid.UUID{},
		nil,
		writerOrDiscard(stdout),
		writerOrDiscard(stderr),
		"codex",
		"login",
	)
}

func (c *Component) AuthStatus(ctx context.Context, stdout io.Writer, stderr io.Writer) error {
	if c == nil || c.runtime == nil {
		return fmt.Errorf("missing component runtime")
	}
	home := c.runtime.ComponentHome()
	runtimeHomePath := c.runtime.RuntimeComponentHomePath()
	if err := PrepareHome(c.config, HomeSpec{
		HostHome:         home.Path,
		RuntimeHome:      runtimeHomePath,
		RuntimeWorkspace: runtimeHomePath,
		SandboxMode:      c.componentConfig.SandboxMode,
	}); err != nil {
		return err
	}
	return c.runtime.Exec(
		ctx,
		"",
		modeluuid.UUID{},
		nil,
		writerOrDiscard(stdout),
		writerOrDiscard(stderr),
		"codex",
		"login",
		"status",
	)
}

func (c *Component) HandleTurn(ctx context.Context, turn component.Turn) (*component.TurnResult, error) {
	if c == nil || c.runner == nil {
		return nil, fmt.Errorf("missing codex runner")
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
	runtimeHomePath := c.runtime.RuntimeComponentHomePath()
	settings, err := c.resolveThreadSettings(ctx, &turn.Thread)
	if err != nil {
		return nil, err
	}
	instructions := turn.Runtime.Instructions()
	instructions.RuntimeNotices = append(instructions.RuntimeNotices, agentcommon.RuntimeNotices(ctx, c.runtime, workspacePath, turn.Thread.ID, c.logf)...)
	bootstrapText, err := codexBootstrap(runtimeWorkspacePath, runtimeHomePath, instructions)
	if err != nil {
		return nil, err
	}
	if err := PrepareHome(c.config, HomeSpec{
		HostHome:         c.runtime.ComponentHome().Path,
		RuntimeHome:      runtimeHomePath,
		RuntimeWorkspace: runtimeWorkspacePath,
		BootstrapText:    bootstrapText,
		SandboxMode:      settings.SandboxMode,
	}); err != nil {
		return nil, err
	}

	stopTyping, err := turn.Runtime.StartChatAction(ctx, message.ChatActionTyping)
	if err == nil && stopTyping != nil {
		defer stopTyping()
	}

	providerThreadID, err := agentcommon.ProviderThreadID(c.registration.ID, turn.Runtime)
	if err != nil {
		return nil, err
	}
	result, runErr := c.runner.RunTurn(ctx, commandRuntime{
		runtime:       c.runtime,
		threadID:      turn.Thread.ID,
		workspacePath: workspacePath,
		commands:      turn.Runtime.Commands(),
	}, outputHandler{runtime: turn.Runtime}, TurnRequest{
		ProviderThreadID: providerThreadID,
		Prompt:           prompt,
		Options:          turnOptionsFromSettings(settings),
	})

	if !settings.KeepRunning {
		agentcommon.StopAfterTurn(c.runtime, workspacePath, turn.Thread.ID, stopAfterTurnTimeout, c.logf)
	}
	if saveErr := agentcommon.BindProviderThreadID(c.registration.ID, turn.Runtime, result.ProviderThreadID); saveErr != nil && runErr == nil {
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
	return &component.TurnResult{
		Final: &coremodel.ThreadMessage{
			Kind:        coremodel.MessageKindAgent,
			ComponentID: c.registration.ID,
			ActorID:     c.registration.Ref(),
			ActorLabel:  "Codex",
			Text:        reply,
		},
	}, nil
}

func turnOptionsFromSettings(settings resolvedThreadSettings) TurnOptions {
	options := TurnOptions{
		SandboxMode: DefaultSandboxMode,
	}
	if mode := strings.TrimSpace(settings.SandboxMode); mode != "" {
		options.SandboxMode = mode
	}
	if settings.ModelSource != "codex" {
		options.Model = settings.Model
	}
	if settings.ReasoningEffortSource != "codex" {
		options.ReasoningEffort = settings.ReasoningEffort
	}
	return options
}

func componentBindConfig(config runtimepkg.BindConfig, cfg *appstate.Config, imageOverride string, runtimeHomePath string) runtimepkg.BindConfig {
	config = config.Clean()
	if strings.TrimSpace(config.Image) == "" && cfg != nil {
		config.Image = strings.TrimSpace(cfg.Docker().Image())
	}
	config.Image = componentImage(firstNonEmpty(imageOverride, config.Image))
	return config.WithEnv(
		"HOME="+runtimeHomePath,
		"CODEX_HOME="+runtimeHomePath,
	)
}

func (c *Component) logf(format string, args ...any) {
	if c != nil && c.logger != nil {
		c.logger.Printf(format, args...)
	}
}

type commandRuntime struct {
	runtime       runtimepkg.ThreadRuntime
	threadID      modeluuid.UUID
	workspacePath string
	commands      commandengine.CommandExecutor
}

func (r commandRuntime) Workspace() string {
	return r.runtime.RuntimeWorkspacePath(r.workspacePath)
}

func (r commandRuntime) Exec(ctx context.Context, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	return r.runtime.Exec(
		ctx,
		r.workspacePath,
		r.threadID,
		r.commands,
		stdout,
		stderr,
		name,
		args...,
	)
}

func (r commandRuntime) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	return r.runtime.CombinedOutput(
		ctx,
		r.workspacePath,
		r.threadID,
		r.commands,
		name,
		args...,
	)
}

type outputHandler struct {
	runtime component.TurnRuntime
}

func (h outputHandler) Send(ctx context.Context, payload message.OutboundPayload) error {
	if h.runtime == nil {
		return nil
	}
	return h.runtime.Send(ctx, payload)
}

func callbackPortOrDefault(value int) int {
	if value > 0 {
		return value
	}
	return DefaultCallbackPort
}

func callbackTimeoutOrDefault(value time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return 10 * time.Minute
}

func writerOrDiscard(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}

func componentImage(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultImage
	}
	return value
}

func codexBootstrap(workspace string, home string, instructions component.TurnInstructions) (string, error) {
	chatProvider := strings.TrimSpace(instructions.ChatProvider)
	if chatProvider == "" {
		chatProvider = "Chat"
	}
	allowedCommandsText := strings.Join(instructions.HostbridgeCommandNames, ", ")
	if strings.TrimSpace(allowedCommandsText) == "" {
		allowedCommandsText = "<none>"
	}
	text, err := codexbootstrap.Text(codexbootstrap.TemplateData{
		Workspace:                 workspace,
		WorkspaceInbox:            workspace + "/inbox",
		CodexHome:                 home,
		ContainerOS:               "linux",
		HostOS:                    goruntime.GOOS,
		ChatProvider:              chatProvider,
		MessagePrefix:             instructions.MessagePrefix,
		KeepRepliesConcise:        instructions.KeepRepliesConcise,
		Binaries:                  allowedCommandsText,
		HostbridgeControlCommands: append([]string(nil), instructions.HostbridgeControlCommands...),
		RuntimeNotices:            append([]string(nil), instructions.RuntimeNotices...),
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}
