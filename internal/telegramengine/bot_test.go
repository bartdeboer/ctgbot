package telegramengine

import (
	"context"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/chatmodel"
	"github.com/bartdeboer/ctgbot/internal/codexengine"
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
	active            *codexengine.ChatSession
	created           *codexengine.ChatSession
	markInitializedID uint
	markErrorValue    string
	markCodexThreadID string
}

func (f *fakeSessionStore) AutoMigrate(ctx context.Context) error { return nil }
func (f *fakeSessionStore) GetActive(ctx context.Context, chatID int64, threadID int) (*codexengine.ChatSession, error) {
	return f.active, nil
}
func (f *fakeSessionStore) Create(ctx context.Context, sess *codexengine.ChatSession) error {
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
func (f *fakeSessionStore) MarkCodexThreadID(ctx context.Context, id uint, threadID string) error {
	f.markCodexThreadID = threadID
	return nil
}

type fakeSessionRunner struct {
	startedChatID   int64
	startedThreadID int
	sentPrompt      string
}

func (f *fakeSessionRunner) StartConversation(ctx context.Context, chatID int64, threadID int, workspaceHostPath string) (*codexengine.ChatSession, error) {
	f.startedChatID = chatID
	f.startedThreadID = threadID
	return &codexengine.ChatSession{
		ChatID:        chatID,
		ThreadID:      threadID,
		Active:        true,
		ContainerName: "ctgbot-test",
		WorkspaceHost: "/tmp/workspace",
	}, nil
}

func (f *fakeSessionRunner) StopConversation(ctx context.Context, conv *codexengine.ChatSession) error {
	return nil
}

func (f *fakeSessionRunner) SendPrompt(ctx context.Context, conv *codexengine.ChatSession, prompt string) (string, error) {
	f.sentPrompt = prompt
	return "reply text", nil
}

func TestHandlePromptAutoStartsConversation(t *testing.T) {
	t.Parallel()

	api := &fakeTelegramAPI{}
	sessions := &fakeSessionStore{}
	executor := &fakeSessionRunner{}
	tb := &TelegramBot{
		API:      api,
		Sessions: sessions,
		Executor: executor,
	}

	u := chatmodel.TelegramUpdate{
		ChatID:    42,
		ThreadID:  7,
		MessageID: 99,
	}

	if err := tb.handlePrompt(context.Background(), u, "hello there"); err != nil {
		t.Fatalf("handlePrompt returned error: %v", err)
	}

	if executor.startedChatID != 42 || executor.startedThreadID != 7 {
		t.Fatalf("auto-started wrong conversation: chat=%d thread=%d", executor.startedChatID, executor.startedThreadID)
	}
	if executor.sentPrompt != "hello there" {
		t.Fatalf("sent prompt = %q, want %q", executor.sentPrompt, "hello there")
	}
	if sessions.created == nil {
		t.Fatalf("expected session to be created")
	}
	if sessions.markInitializedID != 1 {
		t.Fatalf("expected session to be marked initialized, got %d", sessions.markInitializedID)
	}
	if len(api.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(api.messages))
	}
	if api.messages[0].text != "conversation started\ncontainer: ctgbot-test\nworkspace: /tmp/workspace" {
		t.Fatalf("unexpected first message: %q", api.messages[0].text)
	}
	if api.messages[1].text != "reply text" {
		t.Fatalf("unexpected second message: %q", api.messages[1].text)
	}
}
