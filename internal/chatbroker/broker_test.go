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
