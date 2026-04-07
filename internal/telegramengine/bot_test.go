package telegramengine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/appconfig"
	"github.com/bartdeboer/ctgbot/internal/chatbroker"
	"github.com/bartdeboer/ctgbot/internal/chatmodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/go-clistate"
)

type fakeTelegramAPI struct {
	messages []sentMessage
}

type sentMessage struct {
	chatID   int64
	threadID int
	replyTo  int
	text     string
}

func (f *fakeTelegramAPI) Run(ctx context.Context, _ time.Duration, _ func(context.Context, chatmodel.TelegramUpdate)) error {
	return nil
}

func (f *fakeTelegramAPI) SendMessage(ctx context.Context, chatID int64, threadID int, replyTo int, text string) error {
	f.messages = append(f.messages, sentMessage{
		chatID:   chatID,
		threadID: threadID,
		replyTo:  replyTo,
		text:     text,
	})
	return nil
}

type fakeSessionStore struct {
	chat   *chatbroker.Chat
	thread *chatbroker.Thread
}

func (f *fakeSessionStore) AutoMigrate(ctx context.Context) error { return nil }

func (f *fakeSessionStore) FindChat(ctx context.Context, providerType string, providerChatID string) (*chatbroker.Chat, error) {
	return f.chat, nil
}

func (f *fakeSessionStore) GetChatByID(ctx context.Context, id modeluuid.UUID) (*chatbroker.Chat, error) {
	if f.chat != nil && f.chat.ID == id {
		return f.chat, nil
	}
	return nil, nil
}

func (f *fakeSessionStore) EnsureChat(ctx context.Context, providerType string, providerChatID string, label string) (*chatbroker.Chat, error) {
	if f.chat == nil {
		f.chat = &chatbroker.Chat{ID: modeluuid.New(), ProviderType: providerType, ProviderChatID: providerChatID, Label: label, Enabled: true}
	}
	return f.chat, nil
}

func (f *fakeSessionStore) FindThread(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*chatbroker.Thread, error) {
	return f.thread, nil
}

func (f *fakeSessionStore) EnsureThread(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*chatbroker.Thread, error) {
	if f.thread == nil {
		f.thread = &chatbroker.Thread{ID: modeluuid.New(), ChatID: chatID, ProviderThreadID: providerThreadID}
	}
	return f.thread, nil
}

func (f *fakeSessionStore) SaveThread(ctx context.Context, thread *chatbroker.Thread) error {
	f.thread = thread
	return nil
}

type fakeAgent struct {
	sentPrompt  string
	setupCalled bool
}

func (f *fakeAgent) Name() string { return "codex" }

func (f *fakeAgent) SetupEnvironment(ctx context.Context, sbx *sandboxengine.Sandbox) error {
	f.setupCalled = true
	return nil
}

func (f *fakeAgent) HandleTurn(ctx context.Context, sbx *sandboxengine.Sandbox, providerThreadID string, prompt string) (chatbroker.TurnResult, error) {
	f.sentPrompt = prompt
	return chatbroker.TurnResult{Reply: "reply text"}, nil
}

type fakeSandboxManager struct{}

func (f fakeSandboxManager) NewSandbox(name string) *sandboxengine.Sandbox {
	return &sandboxengine.Sandbox{Name: name}
}

func TestHandleUpdateSerializedAutoStartsConversation(t *testing.T) {
	t.Parallel()

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
	if err := cfg.SetChatEnabled(42, true); err != nil {
		t.Fatalf("set chat enabled: %v", err)
	}

	api := &fakeTelegramAPI{}
	sessions := &fakeSessionStore{}
	agent := &fakeAgent{}
	broker := chatbroker.New(cfg, sessions, fakeSandboxManager{}, nil)
	broker.RegisterAgent("codex", agent)
	tb := &TelegramBot{
		API:    api,
		Broker: broker,
		Config: cfg,
	}

	u := chatmodel.TelegramUpdate{
		ChatID:    42,
		ThreadID:  7,
		MessageID: 99,
	}

	if err := tb.handleUpdateSerialized(context.Background(), u, "hello there"); err != nil {
		t.Fatalf("handleUpdateSerialized returned error: %v", err)
	}

	if agent.sentPrompt != "hello there" {
		t.Fatalf("sent prompt = %q, want %q", agent.sentPrompt, "hello there")
	}
	if sessions.chat == nil || sessions.thread == nil {
		t.Fatalf("expected chat/thread mapping to be created")
	}
	if !sessions.thread.Active {
		t.Fatalf("expected thread to be active")
	}
	if !sessions.thread.Initialized {
		t.Fatalf("expected thread to be initialized")
	}
	if sessions.thread.ChatID != sessions.chat.ID {
		t.Fatalf("thread does not reference chat")
	}
	if !agent.setupCalled {
		t.Fatalf("expected environment setup to be called")
	}
	if len(api.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(api.messages))
	}

	wantStart := "conversation started\ncontainer: " + sessions.thread.ContainerName + "\nworkspace: " + filepath.Join(root, "chats", sessions.chat.ID.String(), "workspace")
	if api.messages[0].text != wantStart {
		t.Fatalf("unexpected first message: %q", api.messages[0].text)
	}
	if api.messages[1].text != "reply text" {
		t.Fatalf("unexpected second message: %q", api.messages[1].text)
	}
}
