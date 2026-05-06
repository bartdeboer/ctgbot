package telegram

import (
	"context"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/dbmodel"
)

func TestDebouncerMergesSlidingPrompt(t *testing.T) {
	updates := make(chan dbmodel.TelegramUpdate, 1)
	d := NewDebouncer(20*time.Millisecond, nil, func(ctx context.Context, update dbmodel.TelegramUpdate) {
		updates <- update
	})

	d.HandleUpdate(context.Background(), dbmodel.TelegramUpdate{ChatID: 1, ThreadID: 2, UserID: 3, MessageID: 10, Text: "one"})
	d.HandleUpdate(context.Background(), dbmodel.TelegramUpdate{ChatID: 1, ThreadID: 2, UserID: 3, MessageID: 11, Text: "two"})

	select {
	case update := <-updates:
		if update.MessageID != 11 || update.Text != "one\n\ntwo" {
			t.Fatalf("update = %#v, want merged text and latest message id", update)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for debounced update")
	}
}

func TestDebouncerCommandFlushesPendingChatThread(t *testing.T) {
	updates := make(chan dbmodel.TelegramUpdate, 2)
	d := NewDebouncer(time.Hour, nil, func(ctx context.Context, update dbmodel.TelegramUpdate) {
		updates <- update
	})

	d.HandleUpdate(context.Background(), dbmodel.TelegramUpdate{ChatID: 1, ThreadID: 2, UserID: 3, MessageID: 10, Text: "pending"})
	d.HandleUpdate(context.Background(), dbmodel.TelegramUpdate{ChatID: 1, ThreadID: 2, UserID: 4, MessageID: 11, Text: "/help"})

	first := <-updates
	second := <-updates
	if first.Text != "pending" || second.Text != "/help" {
		t.Fatalf("updates = %#v then %#v, want pending flushed before command", first, second)
	}
}
