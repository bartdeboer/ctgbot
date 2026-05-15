package messagesender

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

type fakeSender struct {
	requests []component.MessageSendRequest
}

func (s *fakeSender) Type() string { return "gmail" }

func (s *fakeSender) SendMessage(ctx context.Context, request component.MessageSendRequest) (component.MessageSendResult, error) {
	_ = ctx
	s.requests = append(s.requests, request)
	return component.MessageSendResult{ID: "sent-1", ThreadID: request.ThreadID}, nil
}

func TestSurfaceSendsMessageWithAttachments(t *testing.T) {
	sender := &fakeSender{}
	engine, err := commandset.NewBoundEngineForSource(commandengine.SourceHostbridge, []commandset.BoundSurface{{
		Surface:       NewSurface("gmail/work", sender),
		ComponentRef:  "gmail/work",
		ComponentType: "gmail",
	}})
	if err != nil {
		t.Fatalf("NewBoundEngineForSource() error = %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(path, []byte("pdf"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result, err := engine.Run(context.Background(), testRequest(), []string{
		"gmail/work", "message", "Monthly report",
		"--to", "sender@example.com",
		"--subject", "Report",
		"--type", "text/html",
		"--attach", path + ";type=application/pdf;name=monthly.pdf",
	})
	if err != nil {
		t.Fatalf("Run(message) error = %v", err)
	}
	if !strings.Contains(result.Text, "message sent") || !strings.Contains(result.Text, "id: sent-1") {
		t.Fatalf("result text = %q, want sent id", result.Text)
	}
	if got, want := len(sender.requests), 1; got != want {
		t.Fatalf("requests = %d, want %d", got, want)
	}
	request := sender.requests[0]
	if got, want := request.Body, "Monthly report"; got != want {
		t.Fatalf("Body = %q, want %q", got, want)
	}
	if got, want := request.ContentType, "text/html"; got != want {
		t.Fatalf("ContentType = %q, want %q", got, want)
	}
	if got, want := len(request.Attachments), 1; got != want {
		t.Fatalf("Attachments = %d, want %d", got, want)
	}
	if got := request.Attachments[0]; got.Filename != "monthly.pdf" || got.ContentType != "application/pdf" || string(got.Content) != "pdf" {
		t.Fatalf("Attachment = %#v, want parsed attachment", got)
	}
}

func testRequest() commandengine.Request {
	return commandengine.Request{Context: commandengine.Context{Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}}}}
}
