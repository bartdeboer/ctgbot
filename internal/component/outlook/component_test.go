package outlook

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

func TestParseMessageUsesSenderAndBodyPreview(t *testing.T) {
	c := &Component{componentConfig: defaultComponentConfig(), mailboxEmail: "me@example.com"}
	received := time.Date(2026, 7, 9, 8, 0, 0, 0, time.UTC)
	record := c.parseMessage(graphMessage{
		ID:                "msg-1",
		ConversationID:    "conv-1",
		InternetMessageID: "<msg-1@example.com>",
		Subject:           "Hello",
		BodyPreview:       "Preview text",
		From:              recipient{EmailAddress: emailAddress{Name: "Sender", Address: "sender@example.com"}},
		ReceivedDateTime:  received,
	})
	if got, want := record.OutlookMessageID, "msg-1"; got != want {
		t.Fatalf("OutlookMessageID = %q, want %q", got, want)
	}
	if got, want := record.MailboxEmail, "me@example.com"; got != want {
		t.Fatalf("MailboxEmail = %q, want %q", got, want)
	}
	if got, want := record.FromEmail, "sender@example.com"; got != want {
		t.Fatalf("FromEmail = %q, want %q", got, want)
	}
	if got, want := record.BodyText, "Preview text"; got != want {
		t.Fatalf("BodyText = %q, want %q", got, want)
	}
	if !record.ReceivedAt.Equal(received) {
		t.Fatalf("ReceivedAt = %v, want %v", record.ReceivedAt, received)
	}
}

func TestStoreIncomingMessageDedupesByOutlookID(t *testing.T) {
	profile := t.TempDir()
	created, err := NewWithOptions(context.Background(), coremodel.Component{ID: modeluuid.New(), Type: Type, Name: "work"}, nil, runtimepkg.Profile{Path: profile}, nil, Options{})
	if err != nil {
		t.Fatalf("NewWithOptions() error = %v", err)
	}
	c := created.(*Component)
	message := graphMessage{ID: "msg-1", Subject: "Hello", From: recipient{EmailAddress: emailAddress{Address: "sender@example.com"}}}
	first, fresh, err := c.storeIncomingMessage(context.Background(), message)
	if err != nil {
		t.Fatalf("storeIncomingMessage() first error = %v", err)
	}
	if !fresh {
		t.Fatalf("first store fresh = false")
	}
	second, fresh, err := c.storeIncomingMessage(context.Background(), message)
	if err != nil {
		t.Fatalf("storeIncomingMessage() second error = %v", err)
	}
	if fresh {
		t.Fatalf("second store fresh = true")
	}
	if first.ID != second.ID {
		t.Fatalf("deduped ID = %q, want %q", second.ID, first.ID)
	}
}

func TestCommandDefinitionsExposeValidationSliceAndStubs(t *testing.T) {
	c := &Component{}
	var patterns []string
	for _, def := range c.CommandDefinitions() {
		patterns = append(patterns, def.Pattern)
	}
	joined := strings.Join(patterns, "\n")
	for _, want := range []string{"status", "query <query>", "fetch <message_id>", "message view <message_id>", "message <text>", "reply <message_id>"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("patterns = %q, want %q", joined, want)
		}
	}
}

func TestWaitForClientWaitsOnMissingAuthUntilContextCancel(t *testing.T) {
	c := &Component{componentConfig: ComponentConfig{PollInterval: "1ms"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.waitForClient(ctx, time.Millisecond)
	if err == nil || err != context.Canceled {
		t.Fatalf("waitForClient() error = %v, want context.Canceled", err)
	}
}
