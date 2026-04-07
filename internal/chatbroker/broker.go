package chatbroker

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appconfig"
	"github.com/bartdeboer/ctgbot/internal/bootstrapassets"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/hostbridgetls"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
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
	Thread  *Thread
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

func (b *Broker) GetActiveSession(ctx context.Context, chatID int64, threadID int) (*Thread, error) {
	if b.Sessions == nil {
		return nil, nil
	}
	return b.resolveThread(ctx, chatID, threadID, false)
}

func (b *Broker) StartSession(ctx context.Context, chatID int64, threadID int, workspace string, replace bool) (*Thread, error) {
	var out *Thread
	err := b.dispatcher().Run(ctx, b.dispatchKey(chatID, threadID), func(runCtx context.Context) error {
		var err error
		out, err = b.startSession(runCtx, chatID, threadID, workspace, replace)
		return err
	})
	return out, err
}

func (b *Broker) startSession(ctx context.Context, chatID int64, threadID int, workspace string, replace bool) (*Thread, error) {
	current, err := b.GetActiveSession(ctx, chatID, threadID)
	if err != nil {
		return nil, err
	}
	if current != nil {
		if !replace {
			return current, nil
		}
		_ = b.newSandbox(current).Remove(ctx)
		if b.Sessions != nil {
			current.Active = false
			current.LastError = "replaced by /new"
			_ = b.Sessions.SaveThread(ctx, current)
		}
	}

	chat, thread, err := b.resolveChatThread(ctx, chatID, threadID, true)
	if err != nil {
		return nil, err
	}
	if chat == nil {
		chat = &Chat{ID: modeluuid.New()}
	}
	if thread == nil {
		thread = &Thread{ID: modeluuid.New(), ChatID: chat.ID}
	}
	conv, err := b.prepareThread(ctx, thread, chatID, threadID, workspace)
	if err != nil {
		return nil, err
	}
	if err := b.prepareEnvironment(ctx, conv); err != nil {
		return nil, err
	}
	if b.Sessions != nil {
		if err := b.Sessions.SaveThread(ctx, conv); err != nil {
			_ = b.newSandbox(conv).Remove(context.Background())
			return nil, err
		}
	}
	return conv, nil
}

func (b *Broker) StopSession(ctx context.Context, chatID int64, threadID int) error {
	return b.dispatcher().Run(ctx, b.dispatchKey(chatID, threadID), func(runCtx context.Context) error {
		return b.stopSession(runCtx, chatID, threadID)
	})
}

func (b *Broker) stopSession(ctx context.Context, chatID int64, threadID int) error {
	conv, err := b.GetActiveSession(ctx, chatID, threadID)
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
	conv.Active = false
	conv.LastError = "stopped by /stop"
	return b.Sessions.SaveThread(ctx, conv)
}

func (b *Broker) PrepareSession(ctx context.Context, conv *Thread) error {
	chatID, err := strconv.ParseInt(strings.TrimSpace(conv.ProviderChatID), 10, 64)
	if err != nil {
		return fmt.Errorf("parse provider chat id: %w", err)
	}
	threadID, err := strconv.Atoi(strings.TrimSpace(conv.ProviderThreadKey))
	if err != nil {
		return fmt.Errorf("parse provider thread key: %w", err)
	}
	return b.dispatcher().Run(ctx, b.dispatchKey(chatID, threadID), func(runCtx context.Context) error {
		return b.prepareSession(runCtx, conv)
	})
}

func (b *Broker) prepareSession(ctx context.Context, conv *Thread) error {
	return b.prepareEnvironment(ctx, conv)
}

func (b *Broker) HandleCommand(ctx context.Context, chatID int64, threadID int, name string, args []string) (string, error) {
	switch name {
	case "new":
		workspace := ""
		if len(args) > 0 {
			workspace = args[0]
		}
		conv, err := b.StartSession(ctx, chatID, threadID, workspace, true)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("conversation started\ncontainer: %s\nworkspace: %s", conv.ContainerName, conv.WorkspaceHost), nil
	case "stop":
		conv, err := b.GetActiveSession(ctx, chatID, threadID)
		if err != nil {
			return "", err
		}
		if conv == nil {
			return "no active conversation", nil
		}
		if err := b.StopSession(ctx, chatID, threadID); err != nil {
			return "", err
		}
		return "conversation stopped", nil
	case "status":
		conv, err := b.GetActiveSession(ctx, chatID, threadID)
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
		out, err = b.handlePrompt(runCtx, chatID, threadID, prompt)
		return err
	})
	return out, err
}

func (b *Broker) handlePrompt(ctx context.Context, chatID int64, threadID int, prompt string) (PromptOutcome, error) {
	conv, err := b.GetActiveSession(ctx, chatID, threadID)
	if err != nil {
		return PromptOutcome{}, err
	}
	started := false
	if conv == nil {
		conv, err = b.startSession(ctx, chatID, threadID, "", false)
		if err != nil {
			return PromptOutcome{}, err
		}
		started = true
	}

	agent, sbx, err := b.ensurePreparedSession(ctx, conv)
	if err != nil {
		return PromptOutcome{}, err
	}
	defer func() {
		if stopErr := sbx.Stop(context.Background()); stopErr != nil {
			b.logf("stop conversation sandbox %s failed: %v", conv.ContainerName, stopErr)
		}
	}()

	result, runErr := agent.HandleTurn(ctx, sbx, conv.AgentThreadID, prompt)
	if result.ProviderThreadID != "" {
		conv.AgentThreadID = result.ProviderThreadID
	}
	if b.Sessions != nil {
		lastErr := ""
		if runErr != nil {
			lastErr = runErr.Error()
		}
		conv.LastError = lastErr
		_ = b.Sessions.SaveThread(ctx, conv)
	}
	return PromptOutcome{
		Thread:  conv,
		Started: started,
		Reply:   result.Reply,
	}, runErr
}

func (b *Broker) ensurePreparedSession(ctx context.Context, conv *Thread) (Agent, *sandboxengine.Sandbox, error) {
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
		if b.Sessions != nil {
			_ = b.Sessions.SaveThread(ctx, conv)
		}
	}
	return agent, sbx, nil
}

func (b *Broker) prepareEnvironment(ctx context.Context, conv *Thread) error {
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
	if b.Sessions != nil {
		return b.Sessions.SaveThread(ctx, conv)
	}
	return nil
}

func (b *Broker) prepareThread(ctx context.Context, thread *Thread, providerChatID int64, providerThreadID int, workspace string) (*Thread, error) {
	if b.Config == nil {
		return nil, fmt.Errorf("missing config")
	}
	if err := b.Config.EnsurePaths(); err != nil {
		return nil, err
	}
	if _, err := b.Config.EnsureChatRuntimePaths(thread.ChatID); err != nil {
		return nil, err
	}
	workspaceHostPath, err := b.resolveWorkspaceHostPath(thread.ChatID, providerChatID, workspace)
	if err != nil {
		return nil, err
	}
	thread.Active = true
	thread.ProviderChatID = strconv.FormatInt(providerChatID, 10)
	thread.ProviderType = b.defaultAgentName()
	thread.ContainerName = b.Config.ChatContainerName(thread.ChatID, thread.ID)
	thread.WorkspaceHost = workspaceHostPath
	thread.HomeHost = b.Config.ChatCodexHomeDirByID(thread.ChatID)
	thread.ContainerWorkspace = b.Config.ContainerWorkspacePath()
	thread.ContainerHome = b.Config.ContainerHomePath()
	thread.Initialized = false
	thread.AgentThreadID = ""
	thread.LastError = ""
	if err := b.newSandbox(thread).Remove(ctx); err != nil {
		b.logf("ignoring stale sandbox cleanup error for %s: %v", thread.ContainerName, err)
	}
	b.logf("thread prepared name=%s workspace=%s", thread.ContainerName, thread.WorkspaceHost)
	return thread, nil
}

func (b *Broker) ensureSandboxRuntime(ctx context.Context, conv *Thread) error {
	if b.Config == nil {
		return fmt.Errorf("missing config")
	}
	chatID, threadID, ok := b.Config.ParseChatContainerName(conv.ContainerName)
	if !ok {
		return fmt.Errorf("parse container name: %s", conv.ContainerName)
	}
	if _, err := b.Config.EnsureChatRuntimePaths(chatID); err != nil {
		return err
	}
	if err := hostbridgetls.EnsureChatClientMaterials(b.Config.HostbridgeTLSRoot(), b.Config.ChatThreadTLSDir(chatID, threadID), conv.ContainerName); err != nil {
		return fmt.Errorf("ensure hostbridge tls client materials: %w", err)
	}
	return nil
}

func (b *Broker) newSandbox(conv *Thread) *sandboxengine.Sandbox {
	sbx := b.sandboxManager().NewSandbox(conv.ContainerName)
	chatID, threadID, _ := b.Config.ParseChatContainerName(conv.ContainerName)
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
		"ctgbot.chat_id":   chatID.String(),
		"ctgbot.thread_id": threadID.String(),
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
			Source:   b.Config.ChatThreadTLSDir(chatID, threadID),
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

func (b *Broker) developerInstructions(conv *Thread) string {
	providerChatID, _ := strconv.ParseInt(strings.TrimSpace(conv.ProviderChatID), 10, 64)
	allowedCommands := append([]string{}, hostbridge.AllowedCommandNames(hostbridge.MergeAllowedCommandSpecs(b.Config.ChatHostbridgeAllowedCommandSpecs(providerChatID)))...)
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

func (b *Broker) resolveChatThread(ctx context.Context, providerChatID int64, providerThreadID int, create bool) (*Chat, *Thread, error) {
	if b.Sessions == nil {
		return nil, nil, nil
	}
	chatID := strconv.FormatInt(providerChatID, 10)
	threadKey := strconv.Itoa(providerThreadID)
	var (
		chat   *Chat
		thread *Thread
		err    error
	)
	if create {
		chat, err = b.Sessions.EnsureChat(ctx, "telegram", chatID, "")
	} else {
		chat, err = b.Sessions.FindChat(ctx, "telegram", chatID)
	}
	if err != nil || chat == nil {
		return chat, nil, err
	}
	if create {
		thread, err = b.Sessions.EnsureThread(ctx, chat.ID, threadKey)
	} else {
		thread, err = b.Sessions.FindThread(ctx, chat.ID, threadKey)
	}
	if err != nil {
		return nil, nil, err
	}
	return chat, thread, nil
}

func (b *Broker) resolveThread(ctx context.Context, providerChatID int64, providerThreadID int, create bool) (*Thread, error) {
	_, thread, err := b.resolveChatThread(ctx, providerChatID, providerThreadID, create)
	if err != nil || thread == nil {
		return thread, err
	}
	if !thread.Active {
		return nil, nil
	}
	return thread, nil
}

func (b *Broker) resolveWorkspaceHostPath(chatID modeluuid.UUID, providerChatID int64, raw string) (string, error) {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		candidate = b.Config.ChatWorkspaceHostPath(providerChatID)
	}
	if candidate == "" {
		candidate = b.Config.DefaultWorkspaceHostPath()
	}
	if candidate != "" {
		return b.Config.ResolveWorkspaceHostPath(candidate)
	}
	workspace := b.Config.ChatWorkspaceDirByID(chatID)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return "", err
	}
	return workspace, nil
}

func (b *Broker) logf(format string, args ...any) {
	if b.Logger != nil {
		b.Logger.Printf(format, args...)
	}
}
