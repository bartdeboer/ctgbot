package chatbroker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/appconfig"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/go-clistate"
)

func TestHandleIncomingMessageRoutesTelegramCommand(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg, err := appconfig.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	if err := cfg.SetChatEnabled(42, true); err != nil {
		t.Fatalf("set chat enabled: %v", err)
	}

	sessions := &fakeBrokerSessionStore{}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)

	result, err := broker.HandleIncomingMessage(context.Background(), IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/help@ctgbot",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Text != helpText {
		t.Fatalf("message text = %q, want %q", result.Messages[0].Text, helpText)
	}
}

func TestHandleIncomingMessageRunsUpgradeCommand(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg, err := appconfig.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	if err := cfg.SetChatEnabled(42, true); err != nil {
		t.Fatalf("set chat enabled: %v", err)
	}
	if err := cfg.SetChatProcessToolsEnabled(42, true); err != nil {
		t.Fatalf("set chat process tools enabled: %v", err)
	}

	process := &fakeProcessActions{}
	sessions := &fakeBrokerSessionStore{}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)
	broker.ProcessActions = process

	result, err := broker.HandleIncomingMessage(context.Background(), IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/upgrade",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if !process.upgradeCalled {
		t.Fatalf("expected upgrade to be called")
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Text != "upgrade completed\ntype /quit to restart" {
		t.Fatalf("message text = %q", result.Messages[0].Text)
	}
}

func TestHandleIncomingMessageRunsQuitCommand(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg, err := appconfig.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	if err := cfg.SetChatEnabled(42, true); err != nil {
		t.Fatalf("set chat enabled: %v", err)
	}
	if err := cfg.SetChatProcessToolsEnabled(42, true); err != nil {
		t.Fatalf("set chat process tools enabled: %v", err)
	}

	process := &fakeProcessActions{}
	sessions := &fakeBrokerSessionStore{}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)
	broker.ProcessActions = process

	result, err := broker.HandleIncomingMessage(context.Background(), IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/quit",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if !process.quitCalled {
		t.Fatalf("expected quit to be called")
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Text != "shutting down ctgbot" {
		t.Fatalf("message text = %q", result.Messages[0].Text)
	}
}

func TestHandleIncomingMessageBlocksUpgradeWithoutProcessTools(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg, err := appconfig.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	if err := cfg.SetChatEnabled(42, true); err != nil {
		t.Fatalf("set chat enabled: %v", err)
	}

	process := &fakeProcessActions{}
	sessions := &fakeBrokerSessionStore{}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)
	broker.ProcessActions = process

	result, err := broker.HandleIncomingMessage(context.Background(), IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/upgrade",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if process.upgradeCalled {
		t.Fatalf("did not expect upgrade to be called")
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Text != "upgrade is not enabled for this chat" {
		t.Fatalf("message text = %q", result.Messages[0].Text)
	}
}

func TestHandleIncomingMessageBlocksQuitWithoutProcessTools(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg, err := appconfig.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	if err := cfg.SetChatEnabled(42, true); err != nil {
		t.Fatalf("set chat enabled: %v", err)
	}

	process := &fakeProcessActions{}
	sessions := &fakeBrokerSessionStore{}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)
	broker.ProcessActions = process

	result, err := broker.HandleIncomingMessage(context.Background(), IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/quit",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if process.quitCalled {
		t.Fatalf("did not expect quit to be called")
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Text != "quit is not enabled for this chat" {
		t.Fatalf("message text = %q", result.Messages[0].Text)
	}
}

func TestHandleIncomingMessageRefreshesActiveConversation(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg, err := appconfig.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	if err := cfg.SetChatEnabled(42, true); err != nil {
		t.Fatalf("set chat enabled: %v", err)
	}

	chatEntry, err := cfg.EnsureProviderChat("telegram", "42", "Test Chat")
	if err != nil {
		t.Fatalf("ensure provider chat: %v", err)
	}
	threadID := modeluuid.New()
	thread := &Thread{
		ID:                 threadID,
		ChatID:             chatEntry.ID,
		ProviderThreadID:   "7",
		AgentProviderType:  "codex",
		ContainerName:      cfg.ChatContainerName(chatEntry.ID, threadID),
		WorkspaceHost:      filepath.Join(root, "workspace"),
		HomeHost:           cfg.ChatCodexHomeDirByID(chatEntry.ID),
		ContainerWorkspace: cfg.ContainerWorkspacePath(),
		ContainerHome:      cfg.ContainerHomePath(),
		AgentThreadID:      "agent-thread-123",
		Initialized:        true,
		Active:             true,
		LastError:          "old error",
	}
	if err := os.MkdirAll(thread.WorkspaceHost, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.MkdirAll(thread.HomeHost, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}

	sessions := &fakeBrokerSessionStore{thread: thread}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)
	broker.RegisterAgent("codex", fakeBrokerAgent{})

	result, err := broker.HandleIncomingMessage(context.Background(), IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/refresh",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Text != "conversation runtime refreshed" {
		t.Fatalf("message text = %q", result.Messages[0].Text)
	}
	if sessions.thread == nil {
		t.Fatalf("expected saved thread")
	}
	if !sessions.thread.Initialized {
		t.Fatalf("expected refreshed thread to be initialized")
	}
	if sessions.thread.AgentThreadID != "agent-thread-123" {
		t.Fatalf("AgentThreadID = %q, want preserved value", sessions.thread.AgentThreadID)
	}
	if sessions.thread.LastError != "" {
		t.Fatalf("LastError = %q, want cleared", sessions.thread.LastError)
	}
}

func TestHandleIncomingMessageRefreshWithoutActiveConversation(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg, err := appconfig.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	if err := cfg.SetChatEnabled(42, true); err != nil {
		t.Fatalf("set chat enabled: %v", err)
	}

	sessions := &fakeBrokerSessionStore{
		thread: &Thread{
			ID:               modeluuid.New(),
			ChatID:           modeluuid.New(),
			ProviderThreadID: "7",
			Active:           false,
		},
	}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)

	result, err := broker.HandleIncomingMessage(context.Background(), IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/refresh",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Text != "no active conversation" {
		t.Fatalf("message text = %q", result.Messages[0].Text)
	}
}

func TestHandleIncomingMessagePurgesActiveConversation(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg, err := appconfig.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	if err := cfg.SetChatEnabled(42, true); err != nil {
		t.Fatalf("set chat enabled: %v", err)
	}

	chatEntry, err := cfg.EnsureProviderChat("telegram", "42", "Test Chat")
	if err != nil {
		t.Fatalf("ensure provider chat: %v", err)
	}
	threadID := modeluuid.New()
	thread := &Thread{
		ID:                 threadID,
		ChatID:             chatEntry.ID,
		ProviderThreadID:   "7",
		AgentProviderType:  "codex",
		ContainerName:      cfg.ChatContainerName(chatEntry.ID, threadID),
		WorkspaceHost:      filepath.Join(root, "workspace"),
		HomeHost:           cfg.ChatCodexHomeDirByID(chatEntry.ID),
		ContainerWorkspace: cfg.ContainerWorkspacePath(),
		ContainerHome:      cfg.ContainerHomePath(),
		AgentThreadID:      "agent-thread-123",
		Initialized:        true,
		Active:             true,
		LastError:          "old error",
	}
	if err := os.MkdirAll(thread.WorkspaceHost, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.MkdirAll(thread.HomeHost, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}

	sessions := &fakeBrokerSessionStore{thread: thread}
	agent := &fakePurgingBrokerAgent{}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)
	broker.RegisterAgent("codex", agent)

	result, err := broker.HandleIncomingMessage(context.Background(), IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/purge",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Text != "conversation purged" {
		t.Fatalf("message text = %q", result.Messages[0].Text)
	}
	if !agent.purgeCalled {
		t.Fatalf("expected purge hook to be called")
	}
	if agent.providerThreadID != "agent-thread-123" {
		t.Fatalf("providerThreadID = %q", agent.providerThreadID)
	}
	if sessions.thread == nil {
		t.Fatalf("expected saved thread")
	}
	if sessions.thread.Active {
		t.Fatalf("expected purged thread to be inactive")
	}
	if sessions.thread.Initialized {
		t.Fatalf("expected purged thread to be uninitialized")
	}
	if sessions.thread.AgentThreadID != "" {
		t.Fatalf("AgentThreadID = %q, want cleared", sessions.thread.AgentThreadID)
	}
	if sessions.thread.LastError != "" {
		t.Fatalf("LastError = %q, want cleared", sessions.thread.LastError)
	}
}

func TestHandleIncomingMessagePurgeWithoutActiveConversation(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg, err := appconfig.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	if err := cfg.SetChatEnabled(42, true); err != nil {
		t.Fatalf("set chat enabled: %v", err)
	}

	sessions := &fakeBrokerSessionStore{
		thread: &Thread{
			ID:               modeluuid.New(),
			ChatID:           modeluuid.New(),
			ProviderThreadID: "7",
			Active:           false,
		},
	}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)

	result, err := broker.HandleIncomingMessage(context.Background(), IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/purge",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Text != "no active conversation" {
		t.Fatalf("message text = %q", result.Messages[0].Text)
	}
}

type fakeBrokerSessionStore struct {
	thread *Thread
}

func (f *fakeBrokerSessionStore) AutoMigrate(ctx context.Context) error { return nil }

func (f *fakeBrokerSessionStore) FindThread(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*Thread, error) {
	return f.thread, nil
}

func (f *fakeBrokerSessionStore) EnsureThread(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*Thread, error) {
	if f.thread == nil {
		f.thread = &Thread{ID: modeluuid.New(), ChatID: chatID, ProviderThreadID: providerThreadID}
	}
	return f.thread, nil
}

func (f *fakeBrokerSessionStore) SaveThread(ctx context.Context, thread *Thread) error {
	f.thread = thread
	return nil
}

type fakeBrokerSandboxManager struct{}

func (f fakeBrokerSandboxManager) NewSandbox(name string) *sandboxengine.Sandbox {
	return &sandboxengine.Sandbox{Name: name}
}

type fakeProcessActions struct {
	upgradeCalled bool
	quitCalled    bool
}

func (f *fakeProcessActions) Upgrade(ctx context.Context) error {
	f.upgradeCalled = true
	return nil
}

func (f *fakeProcessActions) Quit(ctx context.Context) error {
	f.quitCalled = true
	return nil
}

type fakeBrokerAgent struct{}

func (fakeBrokerAgent) Name() string { return "codex" }

func (fakeBrokerAgent) SetupEnvironment(ctx context.Context, sbx *sandboxengine.Sandbox) error {
	return nil
}

func (fakeBrokerAgent) HandleTurn(ctx context.Context, sbx *sandboxengine.Sandbox, providerThreadID string, prompt string) (TurnResult, error) {
	return TurnResult{}, nil
}

type fakePurgingBrokerAgent struct {
	fakeBrokerAgent
	purgeCalled      bool
	providerThreadID string
}

func (f *fakePurgingBrokerAgent) Purge(ctx context.Context, sbx *sandboxengine.Sandbox, providerThreadID string) error {
	f.purgeCalled = true
	f.providerThreadID = providerThreadID
	return nil
}
