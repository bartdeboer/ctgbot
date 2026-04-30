package component

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

type baseComponent struct{ typ string }

func (c baseComponent) Type() string { return c.typ }

type sourceComponent struct{ baseComponent }

func (sourceComponent) RunEvents(ctx context.Context, emit InboundEventEmitter) error { return nil }

type commandComponent struct{ baseComponent }

func (commandComponent) CommandDefinitions() []commandengine.Definition                 { return nil }
func (commandComponent) RegisterCommandHandlers(registry *commandengine.Registry) error { return nil }

type profileComponent struct{ baseComponent }

func (profileComponent) ManagedFiles() []ManagedFile {
	return []ManagedFile{{RelativePath: "auth.json", Required: true, Sensitive: true}}
}

type authComponent struct{ baseComponent }

func (authComponent) Auth(ctx context.Context, req AuthRequest) error { return nil }

type fullComponent struct{ baseComponent }

func (fullComponent) RunEvents(ctx context.Context, emit InboundEventEmitter) error  { return nil }
func (fullComponent) CommandDefinitions() []commandengine.Definition                 { return nil }
func (fullComponent) RegisterCommandHandlers(registry *commandengine.Registry) error { return nil }
func (fullComponent) ManagedFiles() []ManagedFile                                    { return nil }

type agentComponent struct{ baseComponent }

func (agentComponent) HandleMessage(ctx context.Context, message coremodel.ThreadMessage) (*coremodel.ThreadMessage, error) {
	return nil, nil
}

type relayComponent struct{ baseComponent }

func (relayComponent) SendMessage(ctx context.Context, message coremodel.ThreadMessage) error {
	return nil
}

func TestRegistryFiltersComponentsByCapability(t *testing.T) {
	registry := NewRegistry(
		sourceComponent{baseComponent{typ: "source"}},
		commandComponent{baseComponent{typ: "command"}},
		profileComponent{baseComponent{typ: "profile"}},
		authComponent{baseComponent{typ: "auth"}},
		agentComponent{baseComponent{typ: "agent"}},
		relayComponent{baseComponent{typ: "relay"}},
		fullComponent{baseComponent{typ: "full"}},
	)

	if got := len(registry.Components()); got != 7 {
		t.Fatalf("components len = %d, want 7", got)
	}
	if got := len(registry.EventSources()); got != 2 {
		t.Fatalf("event sources len = %d, want 2", got)
	}
	if got := len(registry.CommandSurfaces()); got != 2 {
		t.Fatalf("command surfaces len = %d, want 2", got)
	}
	if got := len(registry.ProfileOwners()); got != 2 {
		t.Fatalf("profile owners len = %d, want 2", got)
	}
	if got := len(registry.Authenticators()); got != 1 {
		t.Fatalf("authenticators len = %d, want 1", got)
	}
	if got := len(registry.Agents()); got != 1 {
		t.Fatalf("agents len = %d, want 1", got)
	}
	if got := len(registry.OutboundRelays()); got != 1 {
		t.Fatalf("outbound relays len = %d, want 1", got)
	}
	if got := len(Capabilities[EventSource](registry)); got != 2 {
		t.Fatalf("generic event source capabilities len = %d, want 2", got)
	}
}

func TestRegistryReturnsCopies(t *testing.T) {
	registry := NewRegistry(baseComponent{typ: "one"})

	components := registry.Components()
	components[0] = baseComponent{typ: "changed"}

	if got := registry.Components()[0].Type(); got != "one" {
		t.Fatalf("registry component type = %q, want one", got)
	}
}

func TestInboundEventCarriesThreadRoutingAndActor(t *testing.T) {
	event := InboundEvent{
		SourceType: "gmail",
		EventType:  "email.received",
		ExternalID: "msg-123",
		Actor: Actor{
			ID:    "alice@example.com",
			Label: "Alice",
		},
		Text:     "hello",
		Metadata: map[string]string{"subject": "Question"},
	}

	if event.SourceType != "gmail" || event.EventType != "email.received" || event.ExternalID != "msg-123" {
		t.Fatalf("unexpected event identity: %#v", event)
	}
	if event.Actor.ID != "alice@example.com" || event.Actor.Label != "Alice" {
		t.Fatalf("unexpected actor: %#v", event.Actor)
	}
	if event.Metadata["subject"] != "Question" {
		t.Fatalf("unexpected metadata: %#v", event.Metadata)
	}
}

func TestManagedFileMetadata(t *testing.T) {
	owner := profileComponent{baseComponent{typ: "profile"}}
	files := owner.ManagedFiles()
	if len(files) != 1 {
		t.Fatalf("managed files len = %d, want 1", len(files))
	}
	if files[0].RelativePath != "auth.json" || !files[0].Required || !files[0].Sensitive {
		t.Fatalf("unexpected managed file: %#v", files[0])
	}
}
