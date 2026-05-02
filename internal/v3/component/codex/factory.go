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
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	v3component "github.com/bartdeboer/ctgbot/internal/v3/component"
	"github.com/bartdeboer/ctgbot/internal/v3/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v3/repository"
	"github.com/bartdeboer/ctgbot/internal/v3/workspaces"
)

const (
	ComponentType        = "codex"
	DefaultImage         = "ctgbot-codex:latest"
	DefaultCallbackPort  = 1455
	DefaultContainerHome = "/profile"
)

type Factory struct {
	Config     *appstate.Config
	Sandboxes  sandboxengine.RuntimeManager
	Workspaces *workspaces.Manager
	Logger     *log.Logger
	Image      string
}

func NewFactory(cfg *appstate.Config, sandboxes sandboxengine.RuntimeManager, ws *workspaces.Manager, logger *log.Logger, image string) *Factory {
	return &Factory{
		Config:     cfg,
		Sandboxes:  sandboxes,
		Workspaces: ws,
		Logger:     logger,
		Image:      strings.TrimSpace(image),
	}
}

func (f *Factory) Type() string {
	return ComponentType
}

func (f *Factory) Create(ctx context.Context, req v3component.CreateRequest) (v3component.Component, error) {
	_ = ctx
	if f.Config == nil {
		return nil, fmt.Errorf("missing config")
	}
	if f.Sandboxes == nil {
		return nil, fmt.Errorf("missing sandbox manager")
	}
	if f.Workspaces == nil {
		return nil, fmt.Errorf("missing workspace manager")
	}
	executor := codexengine.NewSessionExecutor(f.Config, f.Logger)
	return &Component{
		registration: req.Registration,
		home:         req.Home,
		storage:      req.Storage,
		sandboxes:    f.Sandboxes,
		workspaces:   f.Workspaces,
		config:       f.Config,
		logger:       f.Logger,
		image:        componentImage(f.Image),
		executor:     executor,
	}, nil
}

type Component struct {
	registration coremodel.Component
	home         v3component.Home
	storage      repository.Storage
	sandboxes    sandboxengine.RuntimeManager
	workspaces   *workspaces.Manager
	config       *appstate.Config
	logger       *log.Logger
	image        string
	executor     *codexengine.SessionExecutor
}

func (c *Component) Type() string {
	return ComponentType
}

func (c *Component) ManagedFiles() []v3component.ManagedFile {
	return []v3component.ManagedFile{
		{RelativePath: "auth.json", Required: true, Sensitive: true},
		{RelativePath: "config.toml", Required: false, Sensitive: false},
	}
}

func (c *Component) Auth(ctx context.Context, req v3component.AuthRequest) error {
	if req.SandboxManager == nil {
		return fmt.Errorf("missing sandbox manager")
	}
	if strings.TrimSpace(req.Home.HostPath) == "" {
		return fmt.Errorf("missing component home host path")
	}
	containerHome := strings.TrimSpace(req.Home.ContainerPath)
	if containerHome == "" {
		containerHome = DefaultContainerHome
	}
	spec := sandboxengine.NewBuilder(authSandboxName(req.Registration.Ref())).
		Image(componentImage(req.Image)).
		Workdir(containerHome).
		Env([]string{
			"HOME=" + containerHome,
			"CODEX_HOME=" + containerHome,
		}).
		Mounts([]sandboxengine.Mount{{Source: req.Home.HostPath, Target: containerHome}}).
		SecurityOpts([]string{"seccomp=unconfined"}).
		Cmd([]string{"tail", "-f", "/dev/null"}).
		Build()

	sbx := req.SandboxManager.CreateSandbox(spec)
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

func (c *Component) HandleTurn(ctx context.Context, turn v3component.Turn) (*v3component.TurnResult, error) {
	if c == nil || c.executor == nil {
		return nil, fmt.Errorf("missing codex executor")
	}
	prompt := strings.TrimSpace(turn.Inbound.Text)
	if prompt == "" {
		return nil, nil
	}
	if c.storage == nil {
		return nil, fmt.Errorf("missing storage")
	}

	workspaceHost, err := c.workspaces.Ensure(turn.Thread.ID)
	if err != nil {
		return nil, err
	}
	spec, err := c.runtimeSandboxSpec(turn, workspaceHost)
	if err != nil {
		return nil, err
	}
	runtime := c.sandboxes.CreateRuntime(sandboxengine.RuntimeSpec{
		Sandbox:       *spec,
		AgentCommands: turn.Runtime.Commands(),
	})
	defer func() { _ = runtime.Stop(context.Background()) }()

	sbx := runtime.Sandbox()
	if err := c.executor.SetupEnvironment(ctx, sbx); err != nil {
		return nil, err
	}

	stopAction, err := turn.Runtime.StartChatAction(ctx, messenger.ChatActionTyping)
	if err == nil && stopAction != nil {
		defer stopAction()
	}

	threadState, err := c.storage.ThreadComponentStates().GetByThreadAndComponent(ctx, turn.Thread.ID, c.registration.ID)
	if err != nil {
		return nil, err
	}
	providerThreadID := ""
	if threadState != nil {
		providerThreadID = strings.TrimSpace(threadState.ExternalThreadID)
	}

	result, err := c.executor.HandleTurn(ctx, sbx, outputHandler{runtime: turn.Runtime}, providerThreadID, prompt, agentcore.TurnOptions{})
	if saveErr := c.saveThreadState(ctx, turn.Thread.ID, result.ProviderThreadID); saveErr != nil && err == nil {
		err = saveErr
	}
	if err != nil {
		return nil, err
	}
	reply := strings.TrimSpace(result.Reply)
	if reply == "" {
		return nil, nil
	}
	return &v3component.TurnResult{
		Final: &coremodel.ThreadMessage{
			Kind:        coremodel.MessageKindAgent,
			ComponentID: c.registration.ID,
			ActorID:     c.registration.Ref(),
			ActorLabel:  "Codex",
			Text:        reply,
		},
	}, nil
}

func (c *Component) runtimeSandboxSpec(turn v3component.Turn, workspaceHost string) (*sandboxengine.SandboxSpec, error) {
	containerHome := strings.TrimSpace(c.home.ContainerPath)
	if containerHome == "" {
		containerHome = DefaultContainerHome
	}
	containerWorkspace := c.workspaces.ContainerWorkspace()
	bootstrapText, err := codexBootstrap(containerWorkspace, containerHome)
	if err != nil {
		return nil, err
	}
	return sandboxengine.NewBuilder(runtimeSandboxName(c.registration.Ref(), turn.Thread.ID.String())).
		WorkspaceDir(workspaceHost).
		ProfileDir(c.home.HostPath).
		ContainerWorkspace(containerWorkspace).
		ContainerHome(containerHome).
		DeveloperInstructions(bootstrapText).
		Hostname(runtimeSandboxName(c.registration.Ref(), turn.Thread.ID.String())).
		Image(c.image).
		Workdir(containerWorkspace).
		Env([]string{
			"HOME=" + containerHome,
			"CODEX_HOME=" + containerHome,
		}).
		Mounts([]sandboxengine.Mount{
			{Source: c.home.HostPath, Target: containerHome},
			{Source: workspaceHost, Target: containerWorkspace},
		}).
		SecurityOpts([]string{"seccomp=unconfined"}).
		Cmd([]string{"tail", "-f", "/dev/null"}).
		Build(), nil
}

func (c *Component) saveThreadState(ctx context.Context, threadID modeluuid.UUID, providerThreadID string) error {
	providerThreadID = strings.TrimSpace(providerThreadID)
	if threadID.IsNull() || c.storage == nil {
		return nil
	}
	existing, err := c.storage.ThreadComponentStates().GetByThreadAndComponent(ctx, threadID, c.registration.ID)
	if err != nil {
		return err
	}
	if providerThreadID == "" && existing == nil {
		return nil
	}
	state := &coremodel.ThreadComponentState{
		ThreadID:    threadID,
		ComponentID: c.registration.ID,
	}
	if existing != nil {
		state.ID = existing.ID
		state.StateJSON = existing.StateJSON
	}
	state.ExternalThreadID = providerThreadID
	return c.storage.ThreadComponentStates().Save(ctx, state)
}

type outputHandler struct {
	runtime v3component.TurnRuntime
}

func (h outputHandler) Send(ctx context.Context, payload messenger.OutboundPayload) error {
	if h.runtime == nil {
		return nil
	}
	return h.runtime.Send(ctx, payload)
}

func runtimeSandboxName(ref string, threadID string) string {
	return safeName("ctgbot-v3-"+ref+"-"+threadID, "ctgbot-v3-codex")
}

func authSandboxName(ref string) string {
	return safeName("ctgbot-auth-"+ref, "ctgbot-auth-codex")
}

func safeName(value string, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	var b strings.Builder
	prevDash := false
	for _, r := range value {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fallback
	}
	return out
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
