package messaging

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

func TestThreadMatchesQueryMatchesShortID(t *testing.T) {
	thread := ThreadSummary{
		ShortID: "00VHFc",
	}
	if !threadMatchesQuery(thread, "00vhfc") {
		t.Fatal("threadMatchesQuery() did not match ShortID")
	}
}

func TestListMessagesPagesWithoutLoadingFullHistorySemantics(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	chat := &coremodel.Chat{ID: modeluuid.New(), Label: "team", Enabled: true}
	if err := storage.Chats().Save(ctx, chat); err != nil {
		t.Fatal(err)
	}
	thread := &coremodel.Thread{ID: modeluuid.New(), ChatID: chat.ID, Label: "board"}
	if err := storage.Threads().Save(ctx, thread); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC)
	var ids []modeluuid.UUID
	for i, text := range []string{"one", "two", "three", "four", "five"} {
		id := modeluuid.New()
		ids = append(ids, id)
		if err := storage.Messages().Append(ctx, &coremodel.ThreadMessage{ID: id, ChatID: chat.ID, ThreadID: thread.ID, Text: text, CreatedAt: base.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	service := New(storage)
	actor := coremodel.Actor{ID: "agent", Label: "agent"}

	latest, err := service.ListMessages(ctx, actor, thread.ID, ListMessagesRequest{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := messageTexts(latest.Messages); strings.Join(got, ",") != "four,five" {
		t.Fatalf("latest messages = %#v", got)
	}
	if latest.NextCursor != "" {
		t.Fatalf("latest next cursor = %q, want empty", latest.NextCursor)
	}

	next, err := service.ListMessages(ctx, actor, thread.ID, ListMessagesRequest{Cursor: ids[1].String(), Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := messageTexts(next.Messages); strings.Join(got, ",") != "three,four" {
		t.Fatalf("next messages = %#v", got)
	}
	if next.NextCursor == "" {
		t.Fatal("next cursor is empty, want cursor for remaining messages")
	}
}

func messageTexts(messages []coremodel.ThreadMessage) []string {
	out := make([]string, 0, len(messages))
	for _, message := range messages {
		out = append(out, message.Text)
	}
	return out
}
