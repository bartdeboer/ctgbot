package gmail

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
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

func TestMessageCommandSendsThroughGmailComponent(t *testing.T) {
	c := testComponent(t)
	client := &fakeGmailClient{}
	c.clientOverride = client
	engine, err := commandset.NewBoundEngineForSource(commandengine.SourceHostbridge, []commandset.BoundSurface{{
		Surface:       c,
		ComponentRef:  "gmail/work",
		ComponentType: Type,
	}})
	if err != nil {
		t.Fatalf("NewBoundEngineForSource() error = %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(path, []byte("pdf"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	result, err := engine.Run(context.Background(), commandengine.Request{
		Context: commandengine.Context{Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}}},
	}, []string{
		"gmail/work", "message", "Monthly report",
		"--to", "sender@example.com",
		"--subject", "Report",
		"--type", "text/html",
		"--attach", path + ";type=application/pdf;name=monthly.pdf",
	})
	if err != nil {
		t.Fatalf("Run(gmail message) error = %v", err)
	}
	if !strings.Contains(result.Text, "message sent") || !strings.Contains(result.Text, "id: sent-message") {
		t.Fatalf("result text = %q, want sent id", result.Text)
	}
	if got, want := len(client.sent), 1; got != want {
		t.Fatalf("sent messages = %d, want %d", got, want)
	}
	raw, err := base64.RawURLEncoding.DecodeString(client.sent[0].Raw)
	if err != nil {
		t.Fatalf("decode sent raw: %v", err)
	}
	text := string(raw)
	for _, want := range []string{
		"To: sender@example.com\r\n",
		"Subject: Report\r\n",
		"Content-Type: text/html; charset=\"UTF-8\"\r\n",
		"Content-Type: application/pdf; name=\"monthly.pdf\"\r\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("raw sent message = %q, want contains %q", text, want)
		}
	}
}

func TestBuildGmailSendMessageBuildsMultipartWithAttachment(t *testing.T) {
	message, err := buildGmailSendMessage(component.MessageSendRequest{
		To:          []string{"sender@example.com"},
		Subject:     "Report",
		Body:        "<h1>Monthly report</h1>",
		ContentType: "text/html",
		Attachments: []message.Media{{
			Filename:    "report.pdf",
			ContentType: "application/pdf",
			ContentID:   "report",
			Disposition: "inline",
			Content:     []byte("pdf bytes"),
		}},
	})
	if err != nil {
		t.Fatalf("buildGmailSendMessage() error = %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(message.Raw)
	if err != nil {
		t.Fatalf("decode raw error = %v", err)
	}
	text := string(raw)
	for _, want := range []string{
		"Content-Type: multipart/mixed; boundary=\"ctgbot-gmail-boundary\"\r\n",
		"--ctgbot-gmail-boundary\r\n",
		"Content-Type: text/html; charset=\"UTF-8\"\r\n",
		"\r\n<h1>Monthly report</h1>\r\n",
		"Content-Type: application/pdf; name=\"report.pdf\"\r\n",
		"Content-Disposition: inline; filename=\"report.pdf\"\r\n",
		"Content-ID: <report>\r\n",
		"Content-Transfer-Encoding: base64\r\n",
		base64.StdEncoding.EncodeToString([]byte("pdf bytes")),
		"--ctgbot-gmail-boundary--\r\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("raw message = %q, want contains %q", text, want)
		}
	}
}
