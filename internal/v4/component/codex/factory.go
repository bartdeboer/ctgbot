package codex

import (
	"context"
	"fmt"
	"io"
	"log"
	"runtime"
	"strings"
	"time"

	agentcore "github.com/bartdeboer/ctgbot/internal/agent"
	"github.com/bartdeboer/ctgbot/internal/agent/codexengine"
	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/bootstrapassets"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	v4component "github.com/bartdeboer/ctgbot/internal/v4/component"
	"github.com/bartdeboer/ctgbot/internal/v4/coremodel"
)

const (
	ComponentType        = "codex"
	DefaultImage         = "ctgbot-codex:latest"
	DefaultCallbackPort  = 1455
	DefaultContainerHome = "/profile"
)

type Factory struct {
	Config *appstate.Config
	Logger *log.Logger
	Image  string
}

func NewFactory(cfg *appstate.Config, logger *log.Logger, image string) *Factory {
	return &Factory{
		Config: cfg,
		Logger: logger,
		Image:  strings.TrimSpace(image),
	}
}

func (f *Factory) Type() string {
	return ComponentType
}

func (f *Factory) Create(ctx context.Context, req v4component.CreateRequest) (v4component.Component, error) {
	_ = ctx
	if f.Config == nil {
		return nil, fmt.Errorf("missing config")
	}
	if req.Runtime == nil {
		return nil, fmt.Errorf("missing component runtime")
	}
	executor := codexengine.NewSessionExecutor(f.Config, f.Logger)
	return &Component{
		registration: req.Registration,
		home:         req.Home,
		runtime:      req.Runtime,
		config:       f.Config,
		logger:       f.Logger,
		image:        componentImage(f.Image),
		executor:     executor,
	}, nil
}

type Component struct {
	registration coremodel.Component
	home         v4component.Home
	runtime      v4component.Runtime
	config       *appstate.Config
	logger       *log.Logger
	image        string
	executor     *codexengine.SessionExecutor
}

func (c *Component) Type() string {
	return ComponentType
}

func (c *Component) ManagedFiles() []v4component.ManagedFile {
	return []v4component.ManagedFile{
		{RelativePath: "auth.json", Required: true, Sensitive: true},
		{RelativePath: "config.toml", Required: false, Sensitive: false},
	}
}

func (c *Component) Auth(ctx context.Context, req v4component.AuthRequest) error {
	if c == nil || c.runtime == nil {
		return fmt.Errorf("missing component runtime")
	}
	containerHome := resolveContainerHome(req.Home.ContainerPath)
	sbx, err := c.runtime.StartAuth(ctx, v4component.RuntimeAuthRequest{
		Registration: req.Registration,
		Home:         req.Home,
		Image:        componentImage(req.Image),
		Workdir:      containerHome,
		Env: []string{
			"HOME=" + containerHome,
			"CODEX_HOME=" + containerHome,
		},
	})
	if err != nil {
		return err
	}
	if _, err := sbx.Ensure(ctx); err != nil {
		return err
	}
	relay, err := sbx.OpenHTTPRelayPort(ctx, callbackPort(req.CallbackPort), callbackTimeout(req.CallbackTimeout))
	if err != nil {
		return err
	}
	defer relay.Close(context.Background())
	return sbx.Exec(ctx, writerOrDiscard(req.Stdout), writerOrDiscard(req.Stderr), "codex", "login")
}

func (c *Component) HandleTurn(ctx context.Context, turn v4component.Turn) (*v4component.TurnResult, error) {
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
	containerWorkspace := c.runtime.ContainerWorkspace()
	bootstrapText, err := codexBootstrap(containerWorkspace, containerHome)
	if err != nil {
		return nil, err
	}
	runtime, err := c.runtime.StartTurn(ctx, v4component.RuntimeTurnRequest{
		Registration:          c.registration,
		Home:                  c.home,
		Thread:                turn.Thread,
		Image:                 c.image,
		Workdir:               containerWorkspace,
		DeveloperInstructions: bootstrapText,
		Commands:              turn.Runtime.Commands(),
		Env: []string{
			"HOME=" + containerHome,
			"CODEX_HOME=" + containerHome,
		},
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = runtime.Stop(context.Background()) }()

	sbx := runtime.Sandbox()
	if err := c.executor.SetupEnvironment(ctx, sbx); err != nil {
		return nil, err
	}

	stopAction, err := turn.Runtime.StartChatAction(ctx, messenger.ChatActionTyping)
	if err == nil && stopAction != nil {
		defer stopAction()
	}

	componentThreadID, ok, err := turn.Runtime.ComponentThreadID(c.registration.ID)
	if err != nil {
		return nil, err
	}
	providerThreadID := ""
	if ok {
		providerThreadID = strings.TrimSpace(componentThreadID)
	}

	result, err := c.executor.HandleTurn(ctx, sbx, outputHandler{runtime: turn.Runtime}, providerThreadID, prompt, agentcore.TurnOptions{})
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
	return &v4component.TurnResult{
		Final: &coremodel.ThreadMessage{
			Kind:        coremodel.MessageKindAgent,
			ComponentID: c.registration.ID,
			ActorID:     c.registration.Ref(),
			ActorLabel:  "Codex",
			Text:        reply,
		},
	}, nil
}

func (c *Component) bindComponentThreadID(runtime v4component.TurnRuntime, providerThreadID string) error {
	providerThreadID = strings.TrimSpace(providerThreadID)
	if providerThreadID == "" {
		return nil
	}
	if runtime == nil {
		return fmt.Errorf("missing turn runtime")
	}
	return runtime.BindComponentThreadID(c.registration.ID, providerThreadID)
}

type outputHandler struct {
	runtime v4component.TurnRuntime
}

func (h outputHandler) Send(ctx context.Context, payload messenger.OutboundPayload) error {
	if h.runtime == nil {
		return nil
	}
	return h.runtime.Send(ctx, payload)
}

func callbackPort(value int) int {
	if value > 0 {
		return value
	}
	return DefaultCallbackPort
}

func callbackTimeout(value time.Duration) time.Duration {
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
		HostOS:             runtime.GOOS,
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
