package chatbroker

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/bartdeboer/ctgbot/internal/appconfig"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/hostbridgetls"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"gorm.io/gorm"
)

type ChatSession struct {
	ID uint `gorm:"primaryKey"`

	ChatID   int64 `gorm:"index:idx_chat_session_active"`
	ThreadID int   `gorm:"index:idx_chat_session_active"`
	Active   bool  `gorm:"index:idx_chat_session_active"`

	ProviderType     string
	ProviderThreadID string

	ContainerName string
	WorkspaceHost string
	HomeHost      string

	ThreadRuntimeHost  string
	ContainerWorkspace string
	ContainerHome      string

	Initialized bool
	LastError   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type SessionStorage struct {
	DB *gorm.DB
}

func NewSessionStorage(db *gorm.DB) *SessionStorage {
	return &SessionStorage{DB: db}
}

func (s *SessionStorage) AutoMigrate(ctx context.Context) error {
	return s.DB.WithContext(ctx).AutoMigrate(&ChatSession{})
}

func (s *SessionStorage) GetActive(ctx context.Context, chatID int64, threadID int) (*ChatSession, error) {
	var sess ChatSession
	err := s.DB.WithContext(ctx).
		Where("chat_id = ? AND thread_id = ? AND active = ?", chatID, threadID, true).
		Order("id desc").
		First(&sess).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &sess, nil
}

func (s *SessionStorage) Create(ctx context.Context, sess *ChatSession) error {
	return s.DB.WithContext(ctx).Create(sess).Error
}

func (s *SessionStorage) MarkStopped(ctx context.Context, id uint, lastErr string) error {
	return s.DB.WithContext(ctx).
		Model(&ChatSession{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"active":     false,
			"last_error": lastErr,
		}).Error
}

func (s *SessionStorage) MarkInitialized(ctx context.Context, id uint) error {
	return s.DB.WithContext(ctx).
		Model(&ChatSession{}).
		Where("id = ?", id).
		Update("initialized", true).Error
}

func (s *SessionStorage) MarkError(ctx context.Context, id uint, lastErr string) error {
	return s.DB.WithContext(ctx).
		Model(&ChatSession{}).
		Where("id = ?", id).
		Update("last_error", lastErr).Error
}

func (s *SessionStorage) MarkProviderThreadID(ctx context.Context, id uint, threadID string) error {
	return s.DB.WithContext(ctx).
		Model(&ChatSession{}).
		Where("id = ?", id).
		Update("provider_thread_id", threadID).Error
}

type RuntimeContext struct {
	SandboxName         string
	ProfilePath         string
	WorkspacePath       string
	ThreadRuntimePath   string
	ContainerHome       string
	ContainerWorkspace  string
	HostOS              string
	HostbridgeAddr      string
	AllowedHostCommands []string
}

type TurnResult struct {
	Reply            string
	ProviderThreadID string
}

type Agent interface {
	Name() string
	SandboxSpec(rt RuntimeContext) sandboxengine.Spec
	InitSession(ctx context.Context, rt RuntimeContext, sbx sandboxengine.Sandbox) error
	HandleTurn(ctx context.Context, rt RuntimeContext, sbx sandboxengine.Sandbox, providerThreadID string, prompt string) (TurnResult, error)
}

type SessionStore interface {
	AutoMigrate(ctx context.Context) error
	GetActive(ctx context.Context, chatID int64, threadID int) (*ChatSession, error)
	Create(ctx context.Context, sess *ChatSession) error
	MarkStopped(ctx context.Context, id uint, lastErr string) error
	MarkInitialized(ctx context.Context, id uint) error
	MarkError(ctx context.Context, id uint, lastErr string) error
	MarkProviderThreadID(ctx context.Context, id uint, threadID string) error
}

type PromptOutcome struct {
	Session *ChatSession
	Started bool
	Reply   string
}

type Broker struct {
	Config       *appconfig.Config
	Sessions     SessionStore
	Sandboxes    sandboxengine.Manager
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
	current, err := b.GetActiveConversation(ctx, chatID, threadID)
	if err != nil {
		return nil, err
	}
	if current != nil {
		if !replace {
			return current, nil
		}
		_ = b.sandboxManager().Remove(ctx, current.ContainerName)
		_ = b.Sessions.MarkStopped(ctx, current.ID, "replaced by /new")
	}

	conv, err := b.newConversationSession(ctx, chatID, threadID, workspace)
	if err != nil {
		return nil, err
	}
	if b.Sessions == nil {
		return conv, nil
	}
	if err := b.Sessions.Create(ctx, conv); err != nil {
		_ = b.sandboxManager().Remove(ctx, conv.ContainerName)
		return nil, err
	}
	return conv, nil
}

func (b *Broker) StopConversation(ctx context.Context, chatID int64, threadID int) error {
	conv, err := b.GetActiveConversation(ctx, chatID, threadID)
	if err != nil {
		return err
	}
	if conv == nil {
		return nil
	}
	if err := b.sandboxManager().Remove(ctx, conv.ContainerName); err != nil {
		return err
	}
	if b.Sessions == nil {
		return nil
	}
	return b.Sessions.MarkStopped(ctx, conv.ID, "stopped by /stop")
}

func (b *Broker) HandlePrompt(ctx context.Context, chatID int64, threadID int, prompt string) (PromptOutcome, error) {
	conv, err := b.GetActiveConversation(ctx, chatID, threadID)
	if err != nil {
		return PromptOutcome{}, err
	}
	started := false
	if conv == nil {
		conv, err = b.StartConversation(ctx, chatID, threadID, "", false)
		if err != nil {
			return PromptOutcome{}, err
		}
		started = true
	}

	if err := b.ensureSandboxRuntime(ctx, conv); err != nil {
		return PromptOutcome{}, err
	}
	agent, err := b.agent(conv.ProviderType)
	if err != nil {
		return PromptOutcome{}, err
	}
	rt := b.runtimeContext(conv)
	spec := b.decorateSandboxSpec(conv, agent.SandboxSpec(rt))
	sbx, created, err := b.sandboxManager().Ensure(ctx, spec)
	if err != nil {
		return PromptOutcome{}, err
	}
	if created {
		if err := agent.InitSession(ctx, rt, sbx); err != nil {
			return PromptOutcome{}, err
		}
	}
	defer func() {
		if err := b.sandboxManager().Stop(context.Background(), conv.ContainerName); err != nil {
			b.logf("stop conversation sandbox %s failed: %v", conv.ContainerName, err)
		}
	}()

	result, runErr := agent.HandleTurn(ctx, rt, sbx, conv.ProviderThreadID, prompt)
	if result.ProviderThreadID != "" {
		conv.ProviderThreadID = result.ProviderThreadID
	}
	if conv.ID != 0 && !conv.Initialized && runErr == nil {
		conv.Initialized = true
		_ = b.Sessions.MarkInitialized(ctx, conv.ID)
	}
	if conv.ID != 0 {
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
	threadRuntimePath := b.Config.ChatThreadTLSDir(chatID, threadID)
	conv := &ChatSession{
		ChatID:             chatID,
		ThreadID:           threadID,
		Active:             true,
		ProviderType:       b.defaultAgentName(),
		ContainerName:      b.Config.ChatContainerName(chatID, threadID),
		WorkspaceHost:      workspaceHostPath,
		HomeHost:           b.Config.ChatCodexHomeDirByID(chatID),
		ThreadRuntimeHost:  threadRuntimePath,
		ContainerWorkspace: b.Config.ContainerWorkspacePath(),
		ContainerHome:      b.Config.ContainerHomePath(),
	}
	if err := b.sandboxManager().Remove(ctx, conv.ContainerName); err != nil {
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

func (b *Broker) runtimeContext(conv *ChatSession) RuntimeContext {
	allowedCommands := hostbridge.AllowedCommandNames(hostbridge.MergeAllowedCommandSpecs(b.Config.ChatHostbridgeAllowedCommandSpecs(conv.ChatID)))
	return RuntimeContext{
		SandboxName:         conv.ContainerName,
		ProfilePath:         conv.HomeHost,
		WorkspacePath:       conv.WorkspaceHost,
		ThreadRuntimePath:   conv.ThreadRuntimeHost,
		ContainerHome:       conv.ContainerHome,
		ContainerWorkspace:  conv.ContainerWorkspace,
		HostOS:              runtime.GOOS,
		HostbridgeAddr:      b.Config.ContainerHostbridgeTCPAddr(),
		AllowedHostCommands: allowedCommands,
	}
}

func (b *Broker) decorateSandboxSpec(conv *ChatSession, spec sandboxengine.Spec) sandboxengine.Spec {
	spec.SecurityOpts = appendUnique(spec.SecurityOpts, "seccomp=unconfined")
	spec.Labels = copyStringMap(spec.Labels)
	spec.Labels["ctgbot.managed"] = "true"
	spec.Labels["ctgbot.chat_id"] = fmt.Sprintf("%d", conv.ChatID)
	spec.Labels["ctgbot.thread_id"] = fmt.Sprintf("%d", conv.ThreadID)
	spec.Env = appendUnique(spec.Env,
		"HOSTBRIDGE_ADDR="+b.Config.ContainerHostbridgeTCPAddr(),
		"HOSTBRIDGE_TLS_DIR="+b.Config.ContainerHostbridgeTLSDir(),
	)
	spec.Mounts = append(spec.Mounts, sandboxengine.Mount{
		Source:   b.Config.ChatThreadTLSDir(conv.ChatID, conv.ThreadID),
		Target:   b.Config.ContainerHostbridgeTLSDir(),
		ReadOnly: true,
	})
	if runtime.GOOS == "linux" {
		spec.AddHosts = appendUnique(spec.AddHosts, "host.docker.internal:host-gateway")
	}
	return spec
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

func (b *Broker) logf(format string, args ...any) {
	if b.Logger != nil {
		b.Logger.Printf(format, args...)
	}
}

func appendUnique(slice []string, values ...string) []string {
	for _, value := range values {
		if value == "" {
			continue
		}
		found := false
		for _, existing := range slice {
			if existing == value {
				found = true
				break
			}
		}
		if !found {
			slice = append(slice, value)
		}
	}
	return slice
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
