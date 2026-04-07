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

type IncomingMessage struct {
	ProviderType      string
	ProviderChatID    string
	ProviderThreadID  string
	Message           string
	ChatLabel         string
	UserLabel         string
	ProviderMessageID string
}

type OutboundMessage struct {
	Text string
}

type IncomingResult struct {
	Messages []OutboundMessage
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

func (b *Broker) GetActiveSession(ctx context.Context, thread *Thread) (*Thread, error) {
	if b.Sessions == nil || thread == nil {
		return nil, nil
	}
	if !thread.Active {
		return nil, nil
	}
	return thread, nil
}

func (b *Broker) StartSession(ctx context.Context, chat *Chat, thread *Thread, workspace string, replace bool) (*Thread, error) {
	var out *Thread
	err := b.dispatcher().Run(ctx, b.dispatchKey(chat, thread), func(runCtx context.Context) error {
		var err error
		out, err = b.startSession(runCtx, chat, thread, workspace, replace)
		return err
	})
	return out, err
}

func (b *Broker) startSession(ctx context.Context, chat *Chat, thread *Thread, workspace string, replace bool) (*Thread, error) {
	current, err := b.GetActiveSession(ctx, thread)
	if err != nil {
		return nil, err
	}
	if current != nil {
		if !replace {
			return current, nil
		}
		_ = b.newSandbox(chat, current).Remove(ctx)
		if b.Sessions != nil {
			current.Active = false
			current.LastError = "replaced by /new"
			_ = b.Sessions.SaveThread(ctx, current)
		}
	}

	if chat == nil {
		chat = &Chat{ID: modeluuid.New(), ProviderType: "local"}
	}
	if thread == nil {
		thread = &Thread{ID: modeluuid.New(), ChatID: chat.ID}
	}
	conv, err := b.prepareThread(ctx, chat, thread, workspace)
	if err != nil {
		return nil, err
	}
	if err := b.prepareEnvironment(ctx, conv); err != nil {
		return nil, err
	}
	if b.Sessions != nil {
		if err := b.Sessions.SaveThread(ctx, conv); err != nil {
			_ = b.newSandbox(chat, conv).Remove(context.Background())
			return nil, err
		}
	}
	return conv, nil
}

func (b *Broker) StopSession(ctx context.Context, thread *Thread) error {
	if thread == nil {
		return nil
	}
	return b.dispatcher().Run(ctx, b.dispatchKeyByID(thread.ChatID, thread.ID), func(runCtx context.Context) error {
		return b.stopSession(runCtx, thread)
	})
}

func (b *Broker) stopSession(ctx context.Context, conv *Thread) error {
	if conv == nil {
		return nil
	}
	chat, err := b.chatByID(ctx, conv.ChatID)
	if err != nil {
		return err
	}
	if err := b.newSandbox(chat, conv).Remove(ctx); err != nil {
		return err
	}
	if b.Sessions == nil {
		return nil
	}
	conv.Active = false
	conv.LastError = "stopped by /stop"
	return b.Sessions.SaveThread(ctx, conv)
}

func (b *Broker) PrepareSession(ctx context.Context, chat *Chat, conv *Thread) error {
	if conv == nil {
		return fmt.Errorf("missing thread")
	}
	return b.dispatcher().Run(ctx, b.dispatchKey(chat, conv), func(runCtx context.Context) error {
		return b.prepareSession(runCtx, conv)
	})
}

func (b *Broker) HandleIncomingMessage(ctx context.Context, msg IncomingMessage) (IncomingResult, error) {
	text := strings.TrimSpace(msg.Message)
	if text == "" {
		return IncomingResult{}, nil
	}

	chat, thread, err := b.resolveIncomingThread(ctx, msg, true)
	if err != nil {
		return IncomingResult{}, err
	}
	if err := b.persistIncomingChat(ctx, chat, msg); err != nil {
		b.logf("persist incoming chat failed provider=%q chat=%q: %v", msg.ProviderType, msg.ProviderChatID, err)
	}
	if !b.chatEnabled(chat) {
		b.logf("ignoring update from disabled chat provider=%q chat=%q title=%q", msg.ProviderType, msg.ProviderChatID, msg.ChatLabel)
		return IncomingResult{}, nil
	}

	if strings.HasPrefix(text, "/") {
		args := normalizeIncomingCommand(msg.ProviderType, text)
		if len(args) == 0 {
			return IncomingResult{}, nil
		}
		reply, err := b.handleCommand(ctx, chat, thread, args[0], args[1:])
		if err != nil {
			return IncomingResult{
				Messages: []OutboundMessage{{Text: fmt.Sprintf("command error: %v", err)}},
			}, nil
		}
		if strings.TrimSpace(reply) == "" {
			return IncomingResult{}, nil
		}
		return IncomingResult{
			Messages: []OutboundMessage{{Text: reply}},
		}, nil
	}

	outcome, err := b.handlePrompt(ctx, chat, thread, text)
	if err != nil {
		return IncomingResult{
			Messages: []OutboundMessage{{Text: fmt.Sprintf("conversation error: %v", err)}},
		}, nil
	}
	var messages []OutboundMessage
	if outcome.Started && outcome.Thread != nil {
		messages = append(messages, OutboundMessage{
			Text: fmt.Sprintf("conversation started\ncontainer: %s\nworkspace: %s", outcome.Thread.ContainerName, outcome.Thread.WorkspaceHost),
		})
	}
	if strings.TrimSpace(outcome.Reply) != "" {
		messages = append(messages, OutboundMessage{Text: outcome.Reply})
	}
	return IncomingResult{Messages: messages}, nil
}

func (b *Broker) prepareSession(ctx context.Context, conv *Thread) error {
	return b.prepareEnvironment(ctx, conv)
}

func (b *Broker) handleCommand(ctx context.Context, chat *Chat, thread *Thread, name string, args []string) (string, error) {
	switch name {
	case "new":
		workspace := ""
		if len(args) > 0 {
			workspace = args[0]
		}
		conv, err := b.StartSession(ctx, chat, thread, workspace, true)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("conversation started\ncontainer: %s\nworkspace: %s", conv.ContainerName, conv.WorkspaceHost), nil
	case "stop":
		conv, err := b.GetActiveSession(ctx, thread)
		if err != nil {
			return "", err
		}
		if conv == nil {
			return "no active conversation", nil
		}
		if err := b.StopSession(ctx, conv); err != nil {
			return "", err
		}
		return "conversation stopped", nil
	case "status":
		conv, err := b.GetActiveSession(ctx, thread)
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

func (b *Broker) HandlePrompt(ctx context.Context, chat *Chat, thread *Thread, prompt string) (PromptOutcome, error) {
	var out PromptOutcome
	err := b.dispatcher().Run(ctx, b.dispatchKey(chat, thread), func(runCtx context.Context) error {
		var err error
		out, err = b.handlePrompt(runCtx, chat, thread, prompt)
		return err
	})
	return out, err
}

func (b *Broker) handlePrompt(ctx context.Context, chat *Chat, thread *Thread, prompt string) (PromptOutcome, error) {
	conv, err := b.GetActiveSession(ctx, thread)
	if err != nil {
		return PromptOutcome{}, err
	}
	started := false
	if conv == nil {
		conv, err = b.startSession(ctx, chat, thread, "", false)
		if err != nil {
			return PromptOutcome{}, err
		}
		started = true
	}

	agent, sbx, err := b.ensurePreparedSession(ctx, chat, conv)
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

func (b *Broker) ensurePreparedSession(ctx context.Context, chat *Chat, conv *Thread) (Agent, *sandboxengine.Sandbox, error) {
	if err := b.ensureSandboxRuntime(ctx, conv); err != nil {
		return nil, nil, err
	}
	agent, err := b.agent(conv.ProviderType)
	if err != nil {
		return nil, nil, err
	}
	sbx := b.newSandbox(chat, conv)
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
	chat, err := b.chatByID(ctx, conv.ChatID)
	if err != nil {
		return err
	}
	sbx := b.newSandbox(chat, conv)
	if err := agent.SetupEnvironment(ctx, sbx); err != nil {
		return err
	}
	conv.Initialized = true
	if b.Sessions != nil {
		return b.Sessions.SaveThread(ctx, conv)
	}
	return nil
}

func (b *Broker) prepareThread(ctx context.Context, chat *Chat, thread *Thread, workspace string) (*Thread, error) {
	if b.Config == nil {
		return nil, fmt.Errorf("missing config")
	}
	if err := b.Config.EnsurePaths(); err != nil {
		return nil, err
	}
	if _, err := b.Config.EnsureChatRuntimePaths(thread.ChatID); err != nil {
		return nil, err
	}
	workspaceHostPath, err := b.resolveWorkspaceHostPath(chat, workspace)
	if err != nil {
		return nil, err
	}
	thread.Active = true
	thread.ProviderType = b.defaultAgentName()
	thread.ContainerName = b.Config.ChatContainerName(thread.ChatID, thread.ID)
	thread.WorkspaceHost = workspaceHostPath
	thread.HomeHost = b.Config.ChatCodexHomeDirByID(thread.ChatID)
	thread.ContainerWorkspace = b.Config.ContainerWorkspacePath()
	thread.ContainerHome = b.Config.ContainerHomePath()
	thread.Initialized = false
	thread.AgentThreadID = ""
	thread.LastError = ""
	if err := b.newSandbox(chat, thread).Remove(ctx); err != nil {
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

func (b *Broker) newSandbox(chat *Chat, conv *Thread) *sandboxengine.Sandbox {
	sbx := b.sandboxManager().NewSandbox(conv.ContainerName)
	chatID, threadID, _ := b.Config.ParseChatContainerName(conv.ContainerName)
	sbx.WorkspaceDir = conv.WorkspaceHost
	sbx.ProfileDir = conv.HomeHost
	sbx.ContainerWorkspace = conv.ContainerWorkspace
	sbx.ContainerHome = conv.ContainerHome
	sbx.DeveloperInstructions = b.developerInstructions(chat, conv)
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

func (b *Broker) developerInstructions(chat *Chat, conv *Thread) string {
	allowedCommands := append([]string{}, hostbridge.AllowedCommandNames(hostbridge.MergeAllowedCommandSpecs(b.Config.ChatHostbridgeAllowedCommandSpecs(b.providerChatID(chat))))...)
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

func (b *Broker) chatByID(ctx context.Context, id modeluuid.UUID) (*Chat, error) {
	if id.IsNull() {
		return nil, fmt.Errorf("missing chat id")
	}
	if b.Sessions == nil {
		return &Chat{ID: id}, nil
	}
	chat, err := b.Sessions.GetChatByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if chat == nil {
		return nil, fmt.Errorf("chat not found: %s", id.String())
	}
	return chat, nil
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

func (b *Broker) dispatchKey(chat *Chat, thread *Thread) dispatchKey {
	if chat == nil {
		chat = &Chat{}
	}
	if thread == nil {
		thread = &Thread{}
	}
	return b.dispatchKeyByID(chat.ID, thread.ID)
}

func (b *Broker) dispatchKeyByID(chatID modeluuid.UUID, threadID modeluuid.UUID) dispatchKey {
	return dispatchKey{ChatID: chatID, ThreadID: threadID}
}

func (b *Broker) resolveIncomingThread(ctx context.Context, msg IncomingMessage, create bool) (*Chat, *Thread, error) {
	if b.Sessions == nil {
		return nil, nil, nil
	}
	chatLabel := strings.TrimSpace(msg.ChatLabel)
	if chatLabel == "" {
		chatLabel = strings.TrimSpace(msg.UserLabel)
	}
	var (
		chat   *Chat
		thread *Thread
		err    error
	)
	if create {
		chat, err = b.Sessions.EnsureChat(ctx, msg.ProviderType, strings.TrimSpace(msg.ProviderChatID), chatLabel)
	} else {
		chat, err = b.Sessions.FindChat(ctx, msg.ProviderType, strings.TrimSpace(msg.ProviderChatID))
	}
	if err != nil || chat == nil {
		return chat, nil, err
	}
	if create {
		thread, err = b.Sessions.EnsureThread(ctx, chat.ID, strings.TrimSpace(msg.ProviderThreadID))
	} else {
		thread, err = b.Sessions.FindThread(ctx, chat.ID, strings.TrimSpace(msg.ProviderThreadID))
	}
	if err != nil {
		return nil, nil, err
	}
	return chat, thread, nil
}

func (b *Broker) resolveWorkspaceHostPath(chat *Chat, raw string) (string, error) {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		candidate = b.Config.ChatWorkspaceHostPath(b.providerChatID(chat))
	}
	if candidate == "" {
		candidate = b.Config.DefaultWorkspaceHostPath()
	}
	if candidate != "" {
		return b.Config.ResolveWorkspaceHostPath(candidate)
	}
	workspace := b.Config.ChatWorkspaceDirByID(chat.ID)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return "", err
	}
	return workspace, nil
}

func (b *Broker) persistIncomingChat(ctx context.Context, chat *Chat, msg IncomingMessage) error {
	if b.Config == nil || chat == nil || chat.ProviderType != "telegram" {
		return nil
	}
	label := strings.TrimSpace(msg.ChatLabel)
	if label == "" {
		label = strings.TrimSpace(msg.UserLabel)
	}
	if label == "" {
		return nil
	}
	return b.Config.PersistChatID(b.providerChatID(chat), label)
}

func (b *Broker) chatEnabled(chat *Chat) bool {
	if b.Config == nil || chat == nil || chat.ProviderType != "telegram" {
		return true
	}
	return b.Config.ChatEnabled(b.providerChatID(chat))
}

func (b *Broker) providerChatID(chat *Chat) int64 {
	if chat == nil {
		return 0
	}
	providerChatID, err := strconv.ParseInt(strings.TrimSpace(chat.ProviderChatID), 10, 64)
	if err != nil {
		return 0
	}
	return providerChatID
}

func normalizeIncomingCommand(providerType string, text string) []string {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return nil
	}
	fields[0] = strings.TrimPrefix(fields[0], "/")
	if providerType == "telegram" {
		if i := strings.Index(fields[0], "@"); i >= 0 {
			fields[0] = fields[0][:i]
		}
	}
	return fields
}

func (b *Broker) logf(format string, args ...any) {
	if b.Logger != nil {
		b.Logger.Printf(format, args...)
	}
}
