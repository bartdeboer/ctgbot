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
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
)

const (
	Type                 = "codex"
	DefaultImage         = "ctgbot-codex:latest"
	DefaultCallbackPort  = 1455
	DefaultContainerHome = "/profile"
)

func New(ctx context.Context, registration coremodel.Component, profile component.Profile, runtime component.Runtime, home component.Home, storage repository.Storage, cfg *appstate.Config, logger *log.Logger, image string) (component.Component, error) {
	_, _, _, _ = ctx, profile, home, storage
	if cfg == nil {
		return nil, fmt.Errorf("missing config")
	}
	if runtime == nil {
		return nil, fmt.Errorf("missing component runtime")
	}
	return &Component{
		registration: registration,
		runtime:      runtime,
		home:         home,
		config:       cfg,
		logger:       logger,
		image:        componentImage(image),
		executor:     codexengine.NewSessionExecutor(cfg, logger),
	}, nil
}

type Component struct {
	registration coremodel.Component
	runtime      component.Runtime
	home         component.Home
	config       *appstate.Config
	logger       *log.Logger
	image        string
	executor     *codexengine.SessionExecutor
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

func (c *Component) Auth(ctx context.Context, registration coremodel.Component, home component.Home, image string, callbackPort int, callbackTimeout time.Duration, stdout io.Writer, stderr io.Writer) error {
	if c == nil || c.runtime == nil {
		return fmt.Errorf("missing component runtime")
	}
	containerHome := resolveContainerHome(home.ContainerPath)
	sandbox, err := c.runtime.StartAuth(ctx, registration, home, componentImage(image), containerHome, []string{
		"HOME=" + containerHome,
		"CODEX_HOME=" + containerHome,
	})
	if err != nil {
		return err
	}
	if _, err := sandbox.Ensure(ctx); err != nil {
		return err
	}
	relay, err := sandbox.OpenHTTPRelayPort(ctx, callbackPortOrDefault(callbackPort), callbackTimeoutOrDefault(callbackTimeout))
	if err != nil {
		return err
	}
	defer relay.Close(context.Background())
	return sandbox.Exec(ctx, writerOrDiscard(stdout), writerOrDiscard(stderr), "codex", "login")
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

	containerHome := resolveContainerHome(c.home.ContainerPath)
	_, containerWorkspace, err := c.runtime.ThreadWorkspace(turn.Thread.ID)
	if err != nil {
		return nil, err
	}
	bootstrapText, err := codexBootstrap(containerWorkspace, containerHome)
	if err != nil {
		return nil, err
	}

	runtime, err := c.runtime.StartTurn(
		ctx,
		c.registration,
		turn.Thread,
		c.home,
		c.image,
		containerWorkspace,
		[]string{
			"HOME=" + containerHome,
			"CODEX_HOME=" + containerHome,
		},
		bootstrapText,
		turn.Runtime.Commands(),
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = runtime.Stop(context.Background()) }()

	sandbox := runtime.Sandbox()
	if err := c.executor.SetupEnvironment(ctx, sandbox); err != nil {
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

	result, err := c.executor.HandleTurn(ctx, sandbox, outputHandler{runtime: turn.Runtime}, providerThreadID, prompt, agentcore.TurnOptions{})
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

func resolveContainerHome(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultContainerHome
	}
	return value
}

func codexBootstrap(workspace string, home string) (string, error) {
	text, err := bootstrapassets.Text(bootstrapassets.TemplateData{
		Workspace:          workspace,
		WorkspaceInbox:     workspace + "/inbox",
		CodexHome:          home,
		ContainerOS:        "linux",
		HostOS:             goruntime.GOOS,
		ChatProvider:       "Chat",
		MessagePrefix:      "",
		KeepRepliesConcise: false,
		Binaries:           "<none>",
	})
	if err != nil {
		return "", err
	}
	lines := strings.Split(text, "\n")
	var out []string
	for _, line := range lines {
		if strings.Contains(line, "hostbridge") || strings.Contains(line, "Available hostbridge commands") {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n")), nil
}
