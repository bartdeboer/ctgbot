package telegramengine

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/chatbroker"
	"github.com/bartdeboer/ctgbot/internal/chatmodel"
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

type fakeBroker struct {
	session   *chatbroker.ChatSession
	prompt    string
	workspace string
}

func (f *fakeBroker) AutoMigrate(ctx context.Context) error { return nil }

func (f *fakeBroker) GetActiveConversation(ctx context.Context, chatID int64, threadID int) (*chatbroker.ChatSession, error) {
	return f.session, nil
}

func (f *fakeBroker) StartConversation(ctx context.Context, chatID int64, threadID int, workspace string, replace bool) (*chatbroker.ChatSession, error) {
	if f.session == nil || replace {
		if workspace == "" {
			workspace = f.workspace
		}
		f.session = &chatbroker.ChatSession{
			ID:            1,
			ChatID:        chatID,
			ThreadID:      threadID,
			Active:        true,
			ProviderType:  "codex",
			Initialized:   true,
			ContainerName: "ctgbot-42-7",
			WorkspaceHost: workspace,
		}
	}
	return f.session, nil
}

func (f *fakeBroker) StopConversation(ctx context.Context, chatID int64, threadID int) error {
	f.session = nil
	return nil
}

func (f *fakeBroker) HandlePrompt(ctx context.Context, chatID int64, threadID int, prompt string) (chatbroker.PromptOutcome, error) {
	started := false
	if f.session == nil {
		var err error
		f.session, err = f.StartConversation(ctx, chatID, threadID, "", false)
		if err != nil {
			return chatbroker.PromptOutcome{}, err
		}
		started = true
	}
	f.prompt = prompt
	return chatbroker.PromptOutcome{
		Session: f.session,
		Started: started,
		Reply:   "reply text",
	}, nil
}

func TestHandlePromptAutoStartsConversation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	api := &fakeTelegramAPI{}
	broker := &fakeBroker{
		workspace: filepath.Join(root, "chats", "42", "workspace"),
	}
	tb := &TelegramBot{
		API:    api,
		Broker: broker,
	}

	u := chatmodel.TelegramUpdate{
		ChatID:    42,
		ThreadID:  7,
		MessageID: 99,
	}

	if err := tb.handlePrompt(context.Background(), u, "hello there"); err != nil {
		t.Fatalf("handlePrompt returned error: %v", err)
	}

	if broker.prompt != "hello there" {
		t.Fatalf("sent prompt = %q, want %q", broker.prompt, "hello there")
	}
	if broker.session == nil {
		t.Fatalf("expected session to be created")
	}
	if broker.session.ChatID != 42 || broker.session.ThreadID != 7 {
		t.Fatalf("auto-started wrong conversation: chat=%d thread=%d", broker.session.ChatID, broker.session.ThreadID)
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
