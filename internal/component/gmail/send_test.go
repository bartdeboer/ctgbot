package gmail

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/component"
)

func TestBuildGmailSendMessageBuildsRFC822AndThreadID(t *testing.T) {
	message, err := buildGmailSendMessage(component.MessageSendRequest{
		To:        []string{"sender@example.com"},
		Cc:        []string{"copy@example.com"},
		Subject:   "Re: Test subject",
		Body:      "Hi there!",
		ThreadID:  "gmail-thread-1",
		InReplyTo: "<original@example.com>",
	})
	if err != nil {
		t.Fatalf("buildGmailSendMessage() error = %v", err)
	}
	if got, want := message.ThreadId, "gmail-thread-1"; got != want {
		t.Fatalf("ThreadId = %q, want %q", got, want)
	}
	raw, err := base64.RawURLEncoding.DecodeString(message.Raw)
	if err != nil {
		t.Fatalf("decode raw error = %v", err)
	}
	text := string(raw)
	for _, want := range []string{
		"To: sender@example.com\r\n",
		"Cc: copy@example.com\r\n",
		"Subject: Re: Test subject\r\n",
		"In-Reply-To: <original@example.com>\r\n",
		"References: <original@example.com>\r\n",
		"Content-Type: text/plain; charset=\"UTF-8\"\r\n",
		"\r\nHi there!\r\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("raw message = %q, want contains %q", text, want)
		}
	}
}

func TestBuildGmailSendMessageRejectsHeaderInjection(t *testing.T) {
	_, err := buildGmailSendMessage(component.MessageSendRequest{
		To:      []string{"sender@example.com"},
		Subject: "hello\r\nBcc: attacker@example.com",
		Body:    "Hi",
	})
	if err == nil || !strings.Contains(err.Error(), "newlines") {
		t.Fatalf("buildGmailSendMessage() error = %v, want newline rejection", err)
	}
}

func TestSendMessageUsesGmailClient(t *testing.T) {
	c := testComponent(t)
	client := &fakeGmailClient{}
	c.clientOverride = client

	result, err := c.SendMessage(context.Background(), component.MessageSendRequest{
		To:       []string{"sender@example.com"},
		Subject:  "Re: Test",
		Body:     "Hi",
		ThreadID: "thread-1",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if got, want := result.ID, "sent-message"; got != want {
		t.Fatalf("result.ID = %q, want %q", got, want)
	}
	if got, want := len(client.sent), 1; got != want {
		t.Fatalf("sent messages = %d, want %d", got, want)
	}
	if got, want := client.sent[0].ThreadId, "thread-1"; got != want {
		t.Fatalf("sent ThreadId = %q, want %q", got, want)
	}
}
