package gmail

import (
	"context"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	v5component "github.com/bartdeboer/ctgbot/internal/v5/component"
	gmailapi "google.golang.org/api/gmail/v1"
)

func TestManagedFiles(t *testing.T) {
	component := &Component{}
	files := component.ManagedFiles()
	if len(files) != 1 {
		t.Fatalf("managed files len = %d, want 1", len(files))
	}
	if got, want := files[0].RelativePath, "token.json"; got != want {
		t.Fatalf("RelativePath = %q, want %q", got, want)
	}
	if !files[0].Required || !files[0].Sensitive {
		t.Fatalf("unexpected managed file flags: %#v", files[0])
	}
}

func TestGetMessageRequiresServiceAndMessageID(t *testing.T) {
	component := &Component{}
	if _, err := component.GetMessage(context.Background(), "message-1"); err == nil {
		t.Fatal("GetMessage() with nil service succeeded, want error")
	}
}

func TestInboundEventFromMessage(t *testing.T) {
	component := &Component{componentID: modeluuid.New()}
	message := &gmailapi.Message{
		Id:       " msg-123 ",
		ThreadId: " thread-456 ",
		Snippet:  " hello from gmail ",
		Payload: &gmailapi.MessagePart{
			Headers: []*gmailapi.MessagePartHeader{
				{Name: "From", Value: " sender@example.com "},
			},
		},
	}

	event := component.InboundEventFromMessage(message)
	if got, want := event.ComponentID, component.componentID; got != want {
		t.Fatalf("ComponentID = %s, want %s", got, want)
	}
	if got, want := event.ExternalID, "msg-123"; got != want {
		t.Fatalf("ExternalID = %q, want %q", got, want)
	}
	payload := event.Payload
	if got, want := payload.ProviderType, Type; got != want {
		t.Fatalf("ProviderType = %q, want %q", got, want)
	}
	if got, want := payload.ProviderChatID, DefaultUserID; got != want {
		t.Fatalf("ProviderChatID = %q, want %q", got, want)
	}
	if got, want := payload.ProviderThreadID, "thread-456"; got != want {
		t.Fatalf("ProviderThreadID = %q, want %q", got, want)
	}
	if got, want := payload.ProviderMessageID, "msg-123"; got != want {
		t.Fatalf("ProviderMessageID = %q, want %q", got, want)
	}
	if got, want := payload.Text.Text, "hello from gmail"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
	actor := payload.ResolvedActor()
	if got, want := actor.Label, "sender@example.com"; got != want {
		t.Fatalf("Actor.Label = %q, want %q", got, want)
	}
}

func TestInboundEventFromNilMessageUsesDefaults(t *testing.T) {
	component := &Component{componentID: modeluuid.New()}
	event := component.InboundEventFromMessage(nil)
	if event.ComponentID != component.componentID {
		t.Fatalf("ComponentID = %s, want %s", event.ComponentID, component.componentID)
	}
	payload := event.Payload
	if got, want := payload.ProviderType, Type; got != want {
		t.Fatalf("ProviderType = %q, want %q", got, want)
	}
	if got, want := payload.ProviderChatID, DefaultUserID; got != want {
		t.Fatalf("ProviderChatID = %q, want %q", got, want)
	}
	actor := payload.ResolvedActor()
	if got, want := actor.Label, "Email"; got != want {
		t.Fatalf("Actor.Label = %q, want %q", got, want)
	}
}

func TestRunInboundSkeleton(t *testing.T) {
	component := &Component{}
	if err := component.RunInbound(context.Background(), func(ctx context.Context, event v5component.InboundEvent) error {
		_ = ctx
		_ = event
		return nil
	}); err == nil || !strings.Contains(err.Error(), "missing gmail service") {
		t.Fatalf("RunInbound() error = %v, want missing gmail service", err)
	}
}

func TestSenderLabelWithoutHeaders(t *testing.T) {
	if got := senderLabel(&gmailapi.Message{Payload: &gmailapi.MessagePart{Headers: []*gmailapi.MessagePartHeader{{Name: "Subject", Value: "x"}}}}); got != "" {
		t.Fatalf("senderLabel() = %q, want empty", got)
	}
}

func TestResolvedPayloadActorDefaultsToUserRole(t *testing.T) {
	component := &Component{componentID: modeluuid.New()}
	event := component.InboundEventFromMessage(&gmailapi.Message{Id: "1", ThreadId: "2", Snippet: "hi"})
	actor := event.Payload.ResolvedActor()
	if actor.ID == "" || actor.Label == "" {
		t.Fatalf("resolved actor = %#v", actor)
	}
	if len(actor.Roles) != 1 {
		t.Fatalf("roles = %#v", actor.Roles)
	}
}
