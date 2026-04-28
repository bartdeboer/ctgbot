package gmail

import (
	"context"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/v2/component"
	gmailapi "google.golang.org/api/gmail/v1"
)

func TestComponentCapabilities(t *testing.T) {
	gmail := New(nil)
	registry := component.NewRegistry(gmail)

	if gmail.Type() != ComponentType {
		t.Fatalf("Type() = %q, want %q", gmail.Type(), ComponentType)
	}
	if got := len(component.Capabilities[component.EventSource](registry)); got != 1 {
		t.Fatalf("event source capabilities len = %d, want 1", got)
	}
	if got := len(component.Capabilities[component.CommandSurface](registry)); got != 1 {
		t.Fatalf("command surface capabilities len = %d, want 1", got)
	}
	if got := len(component.Capabilities[component.ProfileOwner](registry)); got != 1 {
		t.Fatalf("profile owner capabilities len = %d, want 1", got)
	}
}

func TestManagedFiles(t *testing.T) {
	files := New(nil).ManagedFiles()
	if len(files) != 1 {
		t.Fatalf("managed files len = %d, want 1", len(files))
	}
	if files[0].RelativePath != "token.json" || !files[0].Required || !files[0].Sensitive {
		t.Fatalf("unexpected managed file: %#v", files[0])
	}
}

func TestRegisterCommandHandlersRequiresRegistry(t *testing.T) {
	if err := New(nil).RegisterCommandHandlers(nil); err == nil {
		t.Fatal("RegisterCommandHandlers(nil) succeeded, want error")
	}
	if err := New(nil).RegisterCommandHandlers(commandengine.NewRegistry()); err != nil {
		t.Fatalf("RegisterCommandHandlers() error = %v", err)
	}
}

func TestRunEventsSkeletonRequiresDependencies(t *testing.T) {
	if err := New(nil).RunEvents(context.Background(), func(ctx context.Context, event component.InboundEvent) error { return nil }); err == nil {
		t.Fatal("RunEvents() with nil service succeeded, want error")
	}
}

func TestGetMessageRequiresServiceAndMessageID(t *testing.T) {
	gmail := New(nil)
	if _, err := gmail.GetMessage(context.Background(), "message-1"); err == nil {
		t.Fatal("GetMessage() with nil service succeeded, want error")
	}
}

func TestInboundEventFromMessage(t *testing.T) {
	message := &gmailapi.Message{
		Id:       " msg-123 ",
		ThreadId: " thread-456 ",
		Snippet:  " hello from gmail ",
	}
	event := New(nil).InboundEventFromMessage(message)

	if event.SourceType != ComponentType || event.EventType != EventMessageReceived {
		t.Fatalf("unexpected event type: %#v", event)
	}
	if event.ExternalID != "msg-123" {
		t.Fatalf("ExternalID = %q, want msg-123", event.ExternalID)
	}
	if event.Text != "hello from gmail" {
		t.Fatalf("Text = %q, want hello from gmail", event.Text)
	}
	if event.Metadata["gmail.message_id"] != "msg-123" || event.Metadata["gmail.thread_id"] != "thread-456" {
		t.Fatalf("unexpected metadata: %#v", event.Metadata)
	}
}

func TestRunEventsSkeletonReportsNotImplemented(t *testing.T) {
	gmail := New(&gmailapi.Service{})
	err := gmail.RunEvents(context.Background(), func(ctx context.Context, event component.InboundEvent) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("RunEvents() error = %v, want not implemented", err)
	}
}
