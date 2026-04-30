package telegram

import (
	"context"
	"errors"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/v2/component"
)

type fakeAPI struct {
	updates []Update
	err     error
}

func (f fakeAPI) Run(ctx context.Context, onUpdate func(context.Context, Update) error) error {
	for _, update := range f.updates {
		if err := onUpdate(ctx, update); err != nil {
			return err
		}
	}
	return f.err
}

func TestComponentType(t *testing.T) {
	if got := New(nil).Type(); got != ComponentType {
		t.Fatalf("Type() = %q, want %q", got, ComponentType)
	}
}

func TestRegistryDiscoversTelegramAsEventSource(t *testing.T) {
	registry := component.NewRegistry(New(fakeAPI{}))

	if got := len(component.Capabilities[component.EventSource](registry)); got != 1 {
		t.Fatalf("event source capabilities len = %d, want 1", got)
	}
}

func TestRunEventsEmitsInboundEvent(t *testing.T) {
	api := fakeAPI{updates: []Update{{
		ChatID:    -10042,
		ThreadID:  7,
		MessageID: 99,
		UserID:    123,
		UserLabel: " @bart ",
		IsAdmin:   true,
		Text:      " hello ",
		Metadata:  map[string]string{"telegram.file_count": "2"},
	}}}
	telegram := New(api)

	var events []component.InboundEvent
	if err := telegram.RunEvents(context.Background(), func(ctx context.Context, event component.InboundEvent) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatalf("RunEvents() error = %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	event := events[0]
	if event.SourceType != ComponentType || event.EventType != EventMessageReceived {
		t.Fatalf("unexpected event type: %#v", event)
	}
	if event.ExternalID != "-10042:7:99" {
		t.Fatalf("ExternalID = %q, want -10042:7:99", event.ExternalID)
	}
	if event.ProviderChatID != "-10042" || event.ProviderThreadID != "7" {
		t.Fatalf("unexpected provider identity: %#v", event)
	}
	if event.Actor.ID != "123" || event.Actor.Label != "@bart" || !event.Actor.IsAdmin {
		t.Fatalf("unexpected actor: %#v", event.Actor)
	}
	if event.Text != "hello" {
		t.Fatalf("Text = %q, want hello", event.Text)
	}
	for key, want := range map[string]string{
		"telegram.chat_id":    "-10042",
		"telegram.thread_id":  "7",
		"telegram.message_id": "99",
		"telegram.file_count": "2",
	} {
		if got := event.Metadata[key]; got != want {
			t.Fatalf("metadata[%q] = %q, want %q", key, got, want)
		}
	}
}

func TestRunEventsRejectsMissingDependencies(t *testing.T) {
	if err := New(nil).RunEvents(context.Background(), func(ctx context.Context, event component.InboundEvent) error { return nil }); err == nil {
		t.Fatal("RunEvents() with nil api succeeded, want error")
	}
	if err := New(fakeAPI{}).RunEvents(context.Background(), nil); err == nil {
		t.Fatal("RunEvents() with nil emitter succeeded, want error")
	}
}

func TestRunEventsPropagatesAPIError(t *testing.T) {
	want := errors.New("poll failed")
	err := New(fakeAPI{err: want}).RunEvents(context.Background(), func(ctx context.Context, event component.InboundEvent) error { return nil })
	if !errors.Is(err, want) {
		t.Fatalf("RunEvents() error = %v, want %v", err, want)
	}
}

func TestRunEventsPropagatesEmitterError(t *testing.T) {
	want := errors.New("emit failed")
	err := New(fakeAPI{updates: []Update{{Text: "hello"}}}).RunEvents(context.Background(), func(ctx context.Context, event component.InboundEvent) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("RunEvents() error = %v, want %v", err, want)
	}
}
