package codex

import (
	"context"
	"fmt"
	"io"
	"log"
	goruntime "runtime"
	"strings"
	"time"

	agentcore "github.com/bartdeboer/ctgbot/internal/agent"
	"github.com/bartdeboer/ctgbot/internal/agent/codexengine"
	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/bootstrapassets"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
)

const (
	Type                 = "codex"
	DefaultImage         = "ctgbot-codex:latest"
	DefaultCallbackPort  = 1455
	stopAfterTurnTimeout = 5 * time.Second
)

func New(
	ctx context.Context,
	registration coremodel.Component,
	runtimeFactory v5runtime.Factory,
	home v5runtime.Home,
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
	runtimeHomePath := runtimeFactory.RuntimeComponentHomePath(registration, home)
	runtime := runtimeFactory.Bind(
		registration,
		home,
		componentImage(image),
		[]string{
			"HOME=" + runtimeHomePath,
			"CODEX_HOME=" + runtimeHomePath,
		},
	)
	return &Component{
		registration:     registration,
		runtime:          runtime,
		storage:          storage,
		resolveWorkspace: resolveWorkspace,
		config:           cfg,
		executor:         codexengine.NewSessionExecutor(cfg, logger),
	}, nil
}

type sessionExecutor interface {
	HandleRuntimeTurn(ctx context.Context, runtime codexengine.ExecRuntime, output agentcore.OutputHandler, providerThreadID string, prompt string, options agentcore.TurnOptions) (agentcore.TurnResult, error)
}

type Component struct {
	registration     coremodel.Component
	runtime          v5runtime.Runtime
	storage          repository.Storage
	resolveWorkspace func(context.Context, coremodel.Chat) (string, error)
	config           *appstate.Config
	executor         sessionExecutor
}

func (c *Component) Type() string {
	return Type
}

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{
		{RelativePath: "auth.json", Required: true, Sensitive: true},
		{RelativePath: "config.toml", Required: false, Sensitive: false},
	}
}

func (c *Component) Auth(ctx context.Context, callbackPort int, callbackTimeout time.Duration, stdout io.Writer, stderr io.Writer) error {
	if c == nil || c.runtime == nil {
		return fmt.Errorf("missing component runtime")
	}
	home := c.runtime.ComponentHome()
	runtimeHomePath := c.runtime.RuntimeComponentHomePath()
	if err := codexengine.PrepareConversationHome(c.config, home.Path, runtimeHomePath, runtimeHomePath, ""); err != nil {
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

func (c *Component) HandleTurn(ctx context.Context, turn component.Turn) (*component.TurnResult, error) {
	if c == nil || c.executor == nil {
		return nil, fmt.Errorf("missing codex executor")
	}
	if c.runtime == nil {
		return nil, fmt.Errorf("missing component runtime")
	}
	prompt := strings.TrimSpace(turn.Inbound.Text)
	if prompt == "" {
		return nil, nil
	}

	home := c.runtime.ComponentHome()
	runtimeHomePath := c.runtime.RuntimeComponentHomePath()
	workspacePath := turn.Runtime.WorkspacePath()
	runtimeWorkspacePath := c.runtime.RuntimeWorkspacePath(workspacePath)
	bootstrapText, err := codexBootstrap(runtimeWorkspacePath, runtimeHomePath, turn.Runtime.Instructions())
	if err != nil {
		return nil, err
	}
	if err := codexengine.PrepareConversationHome(c.config, home.Path, runtimeHomePath, runtimeWorkspacePath, bootstrapText); err != nil {
		return nil, err
	}

	stopTyping, err := turn.Runtime.StartChatAction(ctx, messenger.ChatActionTyping)
	if err == nil && stopTyping != nil {
		defer stopTyping()
	}

	componentThreadID, ok, err := turn.Runtime.ComponentThreadID(c.registration.ID)
	if err != nil {
		return nil, err
	}
	providerThreadID := ""
	if ok {
		providerThreadID = strings.TrimSpace(componentThreadID)
	}

	run := commandRuntime{
		runtime:       c.runtime,
		threadID:      turn.Thread.ID,
		workspacePath: workspacePath,
		commands:      turn.Runtime.Commands(),
	}
	options := agentcore.TurnOptions{
		Model:           strings.TrimSpace(turn.Thread.CodexModel),
		ReasoningEffort: strings.TrimSpace(turn.Thread.CodexReasoningEffort),
	}
	result, err := c.executor.HandleRuntimeTurn(ctx, run, outputHandler{runtime: turn.Runtime}, providerThreadID, prompt, options)
	if !turn.Thread.KeepRunning {
		c.stopAfterTurn(workspacePath, turn.Thread.ID)
	}
	if saveErr := c.bindComponentThreadID(turn.Runtime, result.ProviderThreadID); saveErr != nil && err == nil {
		err = saveErr
	}
	if err != nil {
		return nil, err
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

func (c *Component) stopAfterTurn(workspacePath string, threadID modeluuid.UUID) {
	if c == nil || c.runtime == nil {
		return
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), stopAfterTurnTimeout)
	defer cancel()
	if err := c.runtime.Stop(stopCtx, workspacePath, threadID); err != nil {
		log.Printf("codex stop-after-turn failed thread=%s err=%v", threadID, err)
	}
}

func (c *Component) bindComponentThreadID(turnRuntime component.TurnRuntime, providerThreadID string) error {
	providerThreadID = strings.TrimSpace(providerThreadID)
	if providerThreadID == "" {
		return nil
	}
	if turnRuntime == nil {
		return fmt.Errorf("missing turn runtime")
	}
	return turnRuntime.BindComponentThreadID(c.registration.ID, providerThreadID)
}

type outputHandler struct {
	runtime component.TurnRuntime
}

type commandRuntime struct {
	runtime       v5runtime.Runtime
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

func (h outputHandler) Send(ctx context.Context, payload messenger.OutboundPayload) error {
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
	text, err := bootstrapassets.Text(bootstrapassets.TemplateData{
		Workspace:          workspace,
		WorkspaceInbox:     workspace + "/inbox",
		CodexHome:          home,
		ContainerOS:        "linux",
		HostOS:             goruntime.GOOS,
		ChatProvider:       chatProvider,
		MessagePrefix:      instructions.MessagePrefix,
		KeepRepliesConcise: instructions.KeepRepliesConcise,
		Binaries:           allowedCommandsText,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}
