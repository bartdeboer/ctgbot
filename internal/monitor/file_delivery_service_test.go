package monitor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/chatbroker"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/go-clistate"
)

type fakeThreadLookup struct {
	thread *chatbroker.Thread
}

func (f *fakeThreadLookup) FindThreadByID(ctx context.Context, threadID modeluuid.UUID) (*chatbroker.Thread, error) {
	_ = ctx
	if f.thread != nil && f.thread.ID == threadID {
		return f.thread, nil
	}
	return nil, nil
}

type sentDocument struct {
	chatID   int64
	threadID int
	filename string
	caption  string
	content  []byte
}

type fakeTelegramSender struct {
	sent []sentDocument
}

func (f *fakeTelegramSender) SendDocument(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error {
	_ = ctx
	f.sent = append(f.sent, sentDocument{
		chatID:   chatID,
		threadID: threadID,
		filename: filename,
		caption:  caption,
		content:  append([]byte(nil), content...),
	})
	return nil
}

func TestFileDeliveryServiceSendFile(t *testing.T) {
	root := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	chat, err := cfg.EnsureProviderChat("telegram", "42", "Test Chat")
	if err != nil {
		t.Fatalf("ensure provider chat: %v", err)
	}

	thread := &chatbroker.Thread{
		ID:               modeluuid.New(),
		ChatID:           chat.ID,
		ProviderThreadID: "7",
	}
	telegram := &fakeTelegramSender{}
	service := NewFileDeliveryService(cfg, &fakeThreadLookup{thread: thread}, telegram)

	err = service.SendFile(context.Background(), hostbridge.SendFileRequest{
		ChatID:   chat.ID.String(),
		ThreadID: thread.ID.String(),
		Filename: "report.pdf",
		Caption:  "Weekly report",
		Content:  []byte("hello"),
	})
	if err != nil {
		t.Fatalf("SendFile: %v", err)
	}
	if len(telegram.sent) != 1 {
		t.Fatalf("expected one sent document, got %d", len(telegram.sent))
	}
	got := telegram.sent[0]
	if got.chatID != 42 || got.threadID != 7 {
		t.Fatalf("unexpected telegram target: %+v", got)
	}
	if got.filename != "report.pdf" || got.caption != "Weekly report" {
		t.Fatalf("unexpected file metadata: %+v", got)
	}
	if string(got.content) != "hello" {
		t.Fatalf("unexpected content: %q", string(got.content))
	}
}
