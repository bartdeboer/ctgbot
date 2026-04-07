package chatbroker

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appconfig"
	"github.com/bartdeboer/ctgbot/internal/bootstrapassets"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/hostbridgetls"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

type TurnResult struct {
	Reply            string
	ProviderThreadID string
}

type Agent interface {
	Name() string
	SetupEnvironment(ctx context.Context, sbx *sandboxengine.Sandbox) error
	HandleTurn(ctx context.Context, sbx *sandboxengine.Sandbox, providerThreadID string, prompt string) (TurnResult, error)
}

type PromptOutcome struct {
	Session *ChatSession
	Started bool
	Reply   string
}

const helpText = "Commands:\n/new [absolute-host-path]\n/status\n/stop\n/help\n\nAny non-command message is sent to the active Codex conversation."

type Broker struct {
	Config       *appconfig.Config
	Sessions     SessionStore
	Sandboxes    sandboxengine.Manager
	Dispatch     *Dispatcher
	Agents       map[string]Agent
	DefaultAgent string
	Logger       *log.Logger
}

func New(cfg *appconfig.Config, sessions SessionStore, sandboxes sandboxengine.Manager, logger *log.Logger) *Broker {
	if sandboxes == nil {
		sandboxes = &sandboxengine.DockerManager{Logger: logger}
	}
	return &Broker{
		Config:       cfg,
		Sessions:     sessions,
		Sandboxes:    sandboxes,
		Dispatch:     NewDispatcher(),
		Agents:       map[string]Agent{},
		DefaultAgent: "codex",
		Logger:       logger,
	}
}

func (b *Broker) RegisterAgent(name string, agent Agent) {
	if b.Agents == nil {
		b.Agents = map[string]Agent{}
	}
	b.Agents[name] = agent
}

func (b *Broker) AutoMigrate(ctx context.Context) error {
	if b.Sessions == nil {
		return nil
	}
	return b.Sessions.AutoMigrate(ctx)
}

func (b *Broker) GetActiveConversation(ctx context.Context, chatID int64, threadID int) (*ChatSession, error) {
	if b.Sessions == nil {
		return nil, nil
	}
	return b.Sessions.GetActive(ctx, chatID, threadID)
}

func (b *Broker) StartConversation(ctx context.Context, chatID int64, threadID int, workspace string, replace bool) (*ChatSession, error) {
	var out *ChatSession
	err := b.dispatcher().Run(ctx, b.dispatchKey(chatID, threadID), func(runCtx context.Context) error {
		var err error
		out, err = b.startConversationNow(runCtx, chatID, threadID, workspace, replace)
		return err
	})
	return out, err
}

func (b *Broker) startConversationNow(ctx context.Context, chatID int64, threadID int, workspace string, replace bool) (*ChatSession, error) {
	current, err := b.GetActiveConversation(ctx, chatID, threadID)
	if err != nil {
		return nil, err
	}
	if current != nil {
		if !replace {
			return current, nil
		}
		_ = b.newSandbox(current).Remove(ctx)
		if b.Sessions != nil {
			_ = b.Sessions.MarkStopped(ctx, current.ID, "replaced by /new")
		}
	}

	conv, err := b.newConversationSession(ctx, chatID, threadID, workspace)
	if err != nil {
		return nil, err
	}
	if err := b.prepareEnvironment(ctx, conv); err != nil {
		return nil, err
	}
	if b.Sessions == nil {
		return conv, nil
	}
	if err := b.Sessions.Create(ctx, conv); err != nil {
		_ = b.newSandbox(conv).Remove(context.Background())
		return nil, err
	}
	return conv, nil
}

func (b *Broker) StopConversation(ctx context.Context, chatID int64, threadID int) error {
	return b.dispatcher().Run(ctx, b.dispatchKey(chatID, threadID), func(runCtx context.Context) error {
		return b.stopConversationNow(runCtx, chatID, threadID)
	})
}

func (b *Broker) stopConversationNow(ctx context.Context, chatID int64, threadID int) error {
	conv, err := b.GetActiveConversation(ctx, chatID, threadID)
	if err != nil {
		return err
	}
	if conv == nil {
		return nil
	}
	if err := b.newSandbox(conv).Remove(ctx); err != nil {
		return err
	}
	if b.Sessions == nil {
		return nil
	}
	return b.Sessions.MarkStopped(ctx, conv.ID, "stopped by /stop")
}

func (b *Broker) PrepareConversation(ctx context.Context, conv *ChatSession) error {
	return b.dispatcher().Run(ctx, b.dispatchKey(conv.ChatID, conv.ThreadID), func(runCtx context.Context) error {
		return b.prepareEnvironment(runCtx, conv)
	})
}

func (b *Broker) HandleCommand(ctx context.Context, chatID int64, threadID int, name string, args []string) (string, error) {
	switch name {
	case "new":
		workspace := ""
		if len(args) > 0 {
			workspace = args[0]
		}
		conv, err := b.StartConversation(ctx, chatID, threadID, workspace, true)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("conversation started\ncontainer: %s\nworkspace: %s", conv.ContainerName, conv.WorkspaceHost), nil
	case "stop":
		conv, err := b.GetActiveConversation(ctx, chatID, threadID)
		if err != nil {
			return "", err
		}
		if conv == nil {
			return "no active conversation", nil
		}
		if err := b.StopConversation(ctx, chatID, threadID); err != nil {
			return "", err
		}
		return "conversation stopped", nil
	case "status":
		conv, err := b.GetActiveConversation(ctx, chatID, threadID)
		if err != nil {
			return "", err
		}
		if conv == nil {
			return "no active conversation", nil
		}
		msg := fmt.Sprintf(
			"active conversation\ncontainer: %s\nworkspace: %s\ninitialized: %t",
			conv.ContainerName,
			conv.WorkspaceHost,
			conv.Initialized,
		)
		if conv.LastError != "" {
			msg += "\nlast_error: " + conv.LastError
		}
		return msg, nil
	case "help":
		return helpText, nil
	default:
		return "", fmt.Errorf("unknown command %q", name)
	}
}

func (b *Broker) HandlePrompt(ctx context.Context, chatID int64, threadID int, prompt string) (PromptOutcome, error) {
	var out PromptOutcome
	err := b.dispatcher().Run(ctx, b.dispatchKey(chatID, threadID), func(runCtx context.Context) error {
		var err error
		out, err = b.handlePromptNow(runCtx, chatID, threadID, prompt)
		return err
	})
	return out, err
}

func (b *Broker) handlePromptNow(ctx context.Context, chatID int64, threadID int, prompt string) (PromptOutcome, error) {
	conv, err := b.GetActiveConversation(ctx, chatID, threadID)
	if err != nil {
		return PromptOutcome{}, err
	}
	started := false
	if conv == nil {
		conv, err = b.startConversationNow(ctx, chatID, threadID, "", false)
		if err != nil {
			return PromptOutcome{}, err
		}
		started = true
	}

	agent, sbx, err := b.ensurePreparedConversation(ctx, conv)
	if err != nil {
		return PromptOutcome{}, err
	}
	defer func() {
		if stopErr := sbx.Stop(context.Background()); stopErr != nil {
			b.logf("stop conversation sandbox %s failed: %v", conv.ContainerName, stopErr)
		}
	}()

	result, runErr := agent.HandleTurn(ctx, sbx, conv.ProviderThreadID, prompt)
	if result.ProviderThreadID != "" {
		conv.ProviderThreadID = result.ProviderThreadID
	}
	if b.Sessions != nil && conv.ID != 0 {
		if conv.ProviderThreadID != "" {
			_ = b.Sessions.MarkProviderThreadID(ctx, conv.ID, conv.ProviderThreadID)
		}
		lastErr := ""
		if runErr != nil {
			lastErr = runErr.Error()
		}
		_ = b.Sessions.MarkError(ctx, conv.ID, lastErr)
	}
	return PromptOutcome{
		Session: conv,
		Started: started,
		Reply:   result.Reply,
	}, runErr
}

func (b *Broker) ensurePreparedConversation(ctx context.Context, conv *ChatSession) (Agent, *sandboxengine.Sandbox, error) {
	if err := b.ensureSandboxRuntime(ctx, conv); err != nil {
		return nil, nil, err
	}
	agent, err := b.agent(conv.ProviderType)
	if err != nil {
		return nil, nil, err
	}
	sbx := b.newSandbox(conv)
	if !conv.Initialized {
		if err := agent.SetupEnvironment(ctx, sbx); err != nil {
			return nil, nil, err
		}
		conv.Initialized = true
		if b.Sessions != nil && conv.ID != 0 {
			_ = b.Sessions.MarkInitialized(ctx, conv.ID)
		}
	}
	return agent, sbx, nil
}

func (b *Broker) prepareEnvironment(ctx context.Context, conv *ChatSession) error {
	if err := b.ensureSandboxRuntime(ctx, conv); err != nil {
		return err
	}
	agent, err := b.agent(conv.ProviderType)
	if err != nil {
		return err
	}
	sbx := b.newSandbox(conv)
	if err := agent.SetupEnvironment(ctx, sbx); err != nil {
		return err
	}
	conv.Initialized = true
	if b.Sessions != nil && conv.ID != 0 {
		return b.Sessions.MarkInitialized(ctx, conv.ID)
	}
	return nil
}

func (b *Broker) newConversationSession(ctx context.Context, chatID int64, threadID int, workspace string) (*ChatSession, error) {
	if b.Config == nil {
		return nil, fmt.Errorf("missing config")
	}
	if err := b.Config.EnsurePaths(); err != nil {
		return nil, err
	}
	if _, err := b.Config.EnsureChatRuntimePaths(chatID); err != nil {
		return nil, err
	}
	workspaceHostPath, err := b.Config.ResolveChatWorkspaceHostPath(chatID, threadID, workspace)
	if err != nil {
		return nil, err
	}
	conv := &ChatSession{
		ChatID:             chatID,
		ThreadID:           threadID,
		Active:             true,
		ProviderType:       b.defaultAgentName(),
		ContainerName:      b.Config.ChatContainerName(chatID, threadID),
		WorkspaceHost:      workspaceHostPath,
		HomeHost:           b.Config.ChatCodexHomeDirByID(chatID),
		ContainerWorkspace: b.Config.ContainerWorkspacePath(),
		ContainerHome:      b.Config.ContainerHomePath(),
	}
	if err := b.newSandbox(conv).Remove(ctx); err != nil {
		b.logf("ignoring stale sandbox cleanup error for %s: %v", conv.ContainerName, err)
	}
	b.logf("conversation session prepared name=%s workspace=%s", conv.ContainerName, conv.WorkspaceHost)
	return conv, nil
}

func (b *Broker) ensureSandboxRuntime(ctx context.Context, conv *ChatSession) error {
	if b.Config == nil {
		return fmt.Errorf("missing config")
	}
	if _, err := b.Config.EnsureChatRuntimePaths(conv.ChatID); err != nil {
		return err
	}
	if err := hostbridgetls.EnsureChatClientMaterials(b.Config.HostbridgeTLSRoot(), b.Config.ChatThreadTLSDir(conv.ChatID, conv.ThreadID), conv.ContainerName); err != nil {
		return fmt.Errorf("ensure hostbridge tls client materials: %w", err)
	}
	return nil
}

func (b *Broker) newSandbox(conv *ChatSession) *sandboxengine.Sandbox {
	sbx := b.sandboxManager().NewSandbox(conv.ContainerName)
	sbx.WorkspaceDir = conv.WorkspaceHost
	sbx.ProfileDir = conv.HomeHost
	sbx.ContainerWorkspace = conv.ContainerWorkspace
	sbx.ContainerHome = conv.ContainerHome
	sbx.DeveloperInstructions = b.developerInstructions(conv)
	sbx.Hostname = conv.ContainerName
	sbx.Image = b.Config.DockerImage()
	sbx.Workdir = conv.ContainerWorkspace
	sbx.Labels = map[string]string{
		"ctgbot.managed":   "true",
		"ctgbot.chat_id":   fmt.Sprintf("%d", conv.ChatID),
		"ctgbot.thread_id": fmt.Sprintf("%d", conv.ThreadID),
	}
	sbx.Env = []string{
		"HOME=" + conv.ContainerHome,
		"CODEX_HOME=" + conv.ContainerHome,
		"HOSTBRIDGE_ADDR=" + b.Config.ContainerHostbridgeTCPAddr(),
		"HOSTBRIDGE_TLS_DIR=" + b.Config.ContainerHostbridgeTLSDir(),
	}
	sbx.Mounts = []sandboxengine.Mount{
		{Source: conv.WorkspaceHost, Target: conv.ContainerWorkspace},
		{Source: conv.HomeHost, Target: conv.ContainerHome},
		{
			Source:   b.Config.ChatThreadTLSDir(conv.ChatID, conv.ThreadID),
			Target:   b.Config.ContainerHostbridgeTLSDir(),
			ReadOnly: true,
		},
	}
	sbx.SecurityOpts = []string{"seccomp=unconfined"}
	sbx.Cmd = []string{"tail", "-f", "/dev/null"}
	if runtime.GOOS == "linux" {
		sbx.AddHosts = []string{"host.docker.internal:host-gateway"}
	}
	return sbx
}

func (b *Broker) developerInstructions(conv *ChatSession) string {
	allowedCommands := append([]string{}, hostbridge.AllowedCommandNames(hostbridge.MergeAllowedCommandSpecs(b.Config.ChatHostbridgeAllowedCommandSpecs(conv.ChatID)))...)
	sort.Strings(allowedCommands)
	allowedCommandsText := strings.Join(allowedCommands, ", ")
	if strings.TrimSpace(allowedCommandsText) == "" {
		allowedCommandsText = "<none>"
	}
	text, err := bootstrapassets.Text(bootstrapassets.TemplateData{
		Workspace:          conv.ContainerWorkspace,
		CodexHome:          conv.ContainerHome,
		ContainerOS:        "linux",
		HostOS:             runtime.GOOS,
		HostbridgeAddr:     b.Config.ContainerHostbridgeTCPAddr(),
		Binaries:           allowedCommandsText,
		ChatProvider:       "Telegram",
		MessagePrefix:      "🤖",
		KeepRepliesConcise: true,
	})
	if err != nil {
		b.logf("render bootstrap template failed: %v", err)
		return ""
	}
	return text
}

func (b *Broker) agent(name string) (Agent, error) {
	if name == "" {
		name = b.defaultAgentName()
	}
	agent, ok := b.Agents[name]
	if !ok {
		return nil, fmt.Errorf("unknown agent provider %q", name)
	}
	return agent, nil
}

func (b *Broker) defaultAgentName() string {
	if b.DefaultAgent != "" {
		return b.DefaultAgent
	}
	return "codex"
}

func (b *Broker) sandboxManager() sandboxengine.Manager {
	if b.Sandboxes == nil {
		b.Sandboxes = &sandboxengine.DockerManager{Logger: b.Logger}
	}
	return b.Sandboxes
}

func (b *Broker) dispatcher() *Dispatcher {
	if b.Dispatch == nil {
		b.Dispatch = NewDispatcher()
	}
	return b.Dispatch
}

func (b *Broker) dispatchKey(chatID int64, threadID int) dispatchKey {
	return dispatchKey{ChatID: chatID, ThreadID: threadID}
}

func (b *Broker) logf(format string, args ...any) {
	if b.Logger != nil {
		b.Logger.Printf(format, args...)
	}
}
