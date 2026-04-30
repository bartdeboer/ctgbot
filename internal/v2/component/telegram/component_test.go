package telegram

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/dbmodel"
	"github.com/bartdeboer/ctgbot/internal/v2/component"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

type fakeAPI struct {
	updates []dbmodel.TelegramUpdate
	err     error
	sent    []sentMessage
}

type sentMessage struct {
	chatID    int64
	threadID  int
	replyTo   int
	text      string
	parseMode string
}

func (f *fakeAPI) Run(ctx context.Context, pollTimeout time.Duration, onUpdate func(context.Context, dbmodel.TelegramUpdate)) error {
	for _, update := range f.updates {
		onUpdate(ctx, update)
	}
	return f.err
}

func (f *fakeAPI) SendMessage(ctx context.Context, chatID int64, threadID int, replyTo int, text string, parseMode string) error {
	f.sent = append(f.sent, sentMessage{chatID: chatID, threadID: threadID, replyTo: replyTo, text: text, parseMode: parseMode})
	return nil
}

func TestComponentCapabilities(t *testing.T) {
	telegram := New(nil)
	registry := component.NewRegistry(telegram)

	if telegram.Type() != ComponentType {
		t.Fatalf("Type() = %q, want %q", telegram.Type(), ComponentType)
	}
	if got := len(component.Capabilities[component.EventSource](registry)); got != 1 {
		t.Fatalf("event source capabilities len = %d, want 1", got)
	}
	if got := len(component.Capabilities[component.OutboundRelay](registry)); got != 1 {
		t.Fatalf("outbound relay capabilities len = %d, want 1", got)
	}
}

func TestRunEventsEmitsInboundEvent(t *testing.T) {
	api := &fakeAPI{updates: []dbmodel.TelegramUpdate{{
		ChatID:    -10042,
		ThreadID:  7,
		MessageID: 99,
		UserID:    123,
		Username:  "bart",
		Text:      " hello ",
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
	if event.Actor.ID != "123" || event.Actor.Label != "@bart" {
		t.Fatalf("unexpected actor: %#v", event.Actor)
	}
	if event.Text != "hello" {
		t.Fatalf("Text = %q, want hello", event.Text)
	}
	for key, want := range map[string]string{
		"telegram.chat_id":    "-10042",
		"telegram.thread_id":  "7",
		"telegram.message_id": "99",
	} {
		if got := event.Metadata[key]; got != want {
			t.Fatalf("metadata[%q] = %q, want %q", key, got, want)
		}
	}
}

func TestRunEventsSkipsEmptyTextUpdates(t *testing.T) {
	api := &fakeAPI{updates: []dbmodel.TelegramUpdate{{
		ChatID:    -10042,
		ThreadID:  7,
		MessageID: 99,
		UserID:    123,
		Text:      "",
	}}}
	telegram := New(api)

	var events []component.InboundEvent
	if err := telegram.RunEvents(context.Background(), func(ctx context.Context, event component.InboundEvent) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatalf("RunEvents() error = %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events len = %d, want 0", len(events))
	}
}

func TestSendMessageUsesTelegramMetadata(t *testing.T) {
	api := &fakeAPI{}
	telegram := New(api)

	err := telegram.SendMessage(context.Background(), coremodel.ThreadMessage{
		Text:         "hello back",
		MetadataJSON: `{"telegram.chat_id":"-10042","telegram.thread_id":"7"}`,
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if len(api.sent) != 1 {
		t.Fatalf("sent len = %d, want 1", len(api.sent))
	}
	if api.sent[0].chatID != -10042 || api.sent[0].threadID != 7 || api.sent[0].text != "hello back" {
		t.Fatalf("unexpected sent message: %#v", api.sent[0])
	}
}

func TestRunEventsRejectsMissingDependencies(t *testing.T) {
	if err := New(nil).RunEvents(context.Background(), func(ctx context.Context, event component.InboundEvent) error { return nil }); err == nil {
		t.Fatal("RunEvents() with nil api succeeded, want error")
	}
	if err := New(&fakeAPI{}).RunEvents(context.Background(), nil); err == nil {
		t.Fatal("RunEvents() with nil emitter succeeded, want error")
	}
}

func TestRunEventsPropagatesAPIError(t *testing.T) {
	want := errors.New("poll failed")
	err := New(&fakeAPI{err: want}).RunEvents(context.Background(), func(ctx context.Context, event component.InboundEvent) error { return nil })
	if !errors.Is(err, want) {
		t.Fatalf("RunEvents() error = %v, want %v", err, want)
	}
}
