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
	active               *chatbroker.ChatSession
	created              *chatbroker.ChatSession
	markInitializedID    uint
	markErrorValue       string
	markProviderThreadID string
}

func (f *fakeSessionStore) AutoMigrate(ctx context.Context) error { return nil }

func (f *fakeSessionStore) GetActive(ctx context.Context, chatID int64, threadID int) (*chatbroker.ChatSession, error) {
	return f.active, nil
}

func (f *fakeSessionStore) Create(ctx context.Context, sess *chatbroker.ChatSession) error {
	sess.ID = 1
	f.created = sess
	f.active = sess
	return nil
}

func (f *fakeSessionStore) MarkStopped(ctx context.Context, id uint, lastErr string) error {
	return nil
}

func (f *fakeSessionStore) MarkInitialized(ctx context.Context, id uint) error {
	f.markInitializedID = id
	return nil
}

func (f *fakeSessionStore) MarkError(ctx context.Context, id uint, lastErr string) error {
	f.markErrorValue = lastErr
	return nil
}

func (f *fakeSessionStore) MarkProviderThreadID(ctx context.Context, id uint, threadID string) error {
	f.markProviderThreadID = threadID
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

func (f fakeSandboxManager) Stop(ctx context.Context, name string) error {
	return nil
}

func (f fakeSandboxManager) Remove(ctx context.Context, name string) error {
	return nil
}

func TestHandlePromptAutoStartsConversation(t *testing.T) {
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

	if err := tb.handlePrompt(context.Background(), u, "hello there"); err != nil {
		t.Fatalf("handlePrompt returned error: %v", err)
	}

	if agent.sentPrompt != "hello there" {
		t.Fatalf("sent prompt = %q, want %q", agent.sentPrompt, "hello there")
	}
	if sessions.created == nil {
		t.Fatalf("expected session to be created")
	}
	if sessions.created.ChatID != 42 || sessions.created.ThreadID != 7 {
		t.Fatalf("auto-started wrong conversation: chat=%d thread=%d", sessions.created.ChatID, sessions.created.ThreadID)
	}
	if !sessions.created.Initialized {
		t.Fatalf("expected created session to be initialized")
	}
	if !agent.setupCalled {
		t.Fatalf("expected environment setup to be called")
	}
	if len(api.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(api.messages))
	}

	wantStart := "conversation started\ncontainer: ctgbot-42-7\nworkspace: " + filepath.Join(root, "chats", "42", "workspace")
	if api.messages[0].text != wantStart {
		t.Fatalf("unexpected first message: %q", api.messages[0].text)
	}
	if api.messages[1].text != "reply text" {
		t.Fatalf("unexpected second message: %q", api.messages[1].text)
	}
}
