package broker

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/v2/component"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v2/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBrokerRoutesInboundEventIntoThreadMessage(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)

	broker := New(store, component.NewRegistry())
	message, err := broker.RouteInboundEvent(context.Background(), component.InboundEvent{
		SourceType:       "telegram",
		EventType:        "message.received",
		ExternalID:       "telegram:1:2:3",
		ProviderChatID:   "-1003759705932",
		ProviderThreadID: "845",
		Actor:            component.Actor{ID: "13145044", Label: "@bartdeboer"},
		Text:             "hello",
		Metadata:         map[string]string{"provider_thread_id": "845"},
	})
	if err != nil {
		t.Fatalf("route inbound event: %v", err)
	}
	if message.Direction != coremodel.DirectionInbound || message.Kind != coremodel.MessageKindUser {
		t.Fatalf("unexpected message shape: %#v", message)
	}

	chat, err := store.Chats().EnsureProviderChat(context.Background(), "telegram", "-1003759705932")
	if err != nil {
		t.Fatalf("load chat: %v", err)
	}
	if chat == nil || chat.ProviderChatID != "-1003759705932" {
		t.Fatalf("expected chat to be created, got %#v", chat)
	}

	thread, err := store.Threads().EnsureProviderThread(context.Background(), chat.ID, "845")
	if err != nil {
		t.Fatalf("load thread: %v", err)
	}
	if thread == nil || thread.ChatID != chat.ID || thread.ProviderThreadID != "845" {
		t.Fatalf("expected thread to be created, got %#v", thread)
	}

	messages, err := store.Messages().ListByThreadID(context.Background(), thread.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 || messages[0].Text != "hello" || messages[0].SourceType != "telegram" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	if messages[0].MetadataJSON == "" {
		t.Fatal("expected metadata json")
	}
}

func TestBrokerHandleEventRunsAgentStoresOutboundAndRelays(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	relay := &fakeRelay{}
	agent := &fakeAgent{reply: "agent reply"}
	broker := New(store, component.NewRegistry(agent, relay))

	outcome, err := broker.HandleEvent(context.Background(), component.InboundEvent{
		SourceType:       "telegram",
		EventType:        "message.received",
		ExternalID:       "telegram:1:2:4",
		ProviderChatID:   "-1003759705932",
		ProviderThreadID: "845",
		Actor:            component.Actor{ID: "13145044", Label: "@bartdeboer"},
		Text:             "hello agent",
	})
	if err != nil {
		t.Fatalf("handle event: %v", err)
	}
	if outcome.Inbound == nil || outcome.Inbound.Text != "hello agent" {
		t.Fatalf("unexpected inbound outcome: %#v", outcome.Inbound)
	}
	if len(outcome.Outbound) != 1 || outcome.Outbound[0].Text != "agent reply" {
		t.Fatalf("unexpected outbound outcome: %#v", outcome.Outbound)
	}
	if len(relay.sent) != 1 || relay.sent[0].Text != "agent reply" {
		t.Fatalf("unexpected relayed messages: %#v", relay.sent)
	}

	messages, err := store.Messages().ListByThreadID(context.Background(), outcome.Inbound.ThreadID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected inbound and outbound messages, got %#v", messages)
	}
	if messages[0].Direction != coremodel.DirectionInbound || messages[1].Direction != coremodel.DirectionOutbound {
		t.Fatalf("unexpected message directions: %#v", messages)
	}
}

func newTestStore(t *testing.T) repository.Storage {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "broker-v2.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	store := repository.NewGORM(db)
	if err := store.AutoMigrate(context.Background()); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return store
}

type fakeAgent struct {
	reply string
}

var _ component.Agent = (*fakeAgent)(nil)

func (a *fakeAgent) Type() string { return "fake-agent" }

func (a *fakeAgent) HandleMessage(_ context.Context, message coremodel.ThreadMessage) (*coremodel.ThreadMessage, error) {
	return &coremodel.ThreadMessage{
		Kind:       coremodel.MessageKindAgent,
		SourceType: a.Type(),
		ActorID:    a.Type(),
		ActorLabel: "Fake Agent",
		Text:       a.reply,
	}, nil
}

type fakeRelay struct {
	sent []coremodel.ThreadMessage
}

var _ component.OutboundRelay = (*fakeRelay)(nil)

func (r *fakeRelay) Type() string { return "fake-relay" }

func (r *fakeRelay) SendMessage(_ context.Context, message coremodel.ThreadMessage) error {
	r.sent = append(r.sent, message)
	return nil
}
