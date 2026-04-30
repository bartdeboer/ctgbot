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
	enableProviderChat(t, store, "telegram", "-1003759705932")

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
	enableProviderChat(t, store, "telegram", "-1003759705932")
	relay := &fakeRelay{}
	agent := &fakeAgent{reply: "agent reply"}
	broker := New(store, component.NewRegistry(agent, relay))
	broker.DefaultChatComponents = []coremodel.ChatComponent{
		{ComponentType: agent.Type(), ProfileName: "default", Enabled: true},
		{ComponentType: relay.Type(), ProfileName: "default", Enabled: true},
	}

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

func TestBrokerOnlyRunsBoundComponents(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	enableProviderChat(t, store, "telegram", "-1003759705932")
	boundAgent := &fakeAgent{typ: "bound-agent", reply: "bound reply"}
	unboundAgent := &fakeAgent{typ: "unbound-agent", reply: "unbound reply"}
	boundRelay := &fakeRelay{typ: "bound-relay"}
	unboundRelay := &fakeRelay{typ: "unbound-relay"}
	broker := New(store, component.NewRegistry(boundAgent, unboundAgent, boundRelay, unboundRelay))
	broker.DefaultChatComponents = []coremodel.ChatComponent{
		{ComponentType: boundAgent.Type(), ProfileName: "default", Enabled: true},
		{ComponentType: boundRelay.Type(), ProfileName: "default", Enabled: true},
	}

	outcome, err := broker.HandleEvent(context.Background(), component.InboundEvent{
		SourceType:       "telegram",
		EventType:        "message.received",
		ExternalID:       "telegram:1:2:5",
		ProviderChatID:   "-1003759705932",
		ProviderThreadID: "845",
		Text:             "hello bound agent",
	})
	if err != nil {
		t.Fatalf("handle event: %v", err)
	}
	if len(outcome.Outbound) != 1 || outcome.Outbound[0].Text != "bound reply" {
		t.Fatalf("unexpected outbound: %#v", outcome.Outbound)
	}
	if boundAgent.calls != 1 || unboundAgent.calls != 0 {
		t.Fatalf("unexpected agent calls: bound=%d unbound=%d", boundAgent.calls, unboundAgent.calls)
	}
	if len(boundRelay.sent) != 1 || len(unboundRelay.sent) != 0 {
		t.Fatalf("unexpected relay calls: bound=%d unbound=%d", len(boundRelay.sent), len(unboundRelay.sent))
	}
}

func TestBrokerBlocksDisabledChats(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	relay := &fakeRelay{}
	agent := &fakeAgent{reply: "agent reply"}
	broker := New(store, component.NewRegistry(agent, relay))
	broker.DefaultChatComponents = []coremodel.ChatComponent{
		{ComponentType: agent.Type(), ProfileName: "default", Enabled: true},
		{ComponentType: relay.Type(), ProfileName: "default", Enabled: true},
	}

	outcome, err := broker.HandleEvent(context.Background(), component.InboundEvent{
		SourceType:       "telegram",
		EventType:        "message.received",
		ExternalID:       "telegram:1:2:6",
		ProviderChatID:   "-1003759705932",
		ProviderThreadID: "845",
		Text:             "blocked",
	})
	if err != nil {
		t.Fatalf("handle event: %v", err)
	}
	if !outcome.Blocked || outcome.Inbound != nil || len(outcome.Outbound) != 0 {
		t.Fatalf("expected blocked empty outcome, got %#v", outcome)
	}
	if agent.calls != 0 || len(relay.sent) != 0 {
		t.Fatalf("disabled chat should not call components: agent=%d relay=%d", agent.calls, len(relay.sent))
	}

	chat, err := store.Chats().EnsureProviderChat(context.Background(), "telegram", "-1003759705932")
	if err != nil {
		t.Fatalf("load chat: %v", err)
	}
	thread, err := store.Threads().EnsureProviderThread(context.Background(), chat.ID, "845")
	if err != nil {
		t.Fatalf("load thread: %v", err)
	}
	messages, err := store.Messages().ListByThreadID(context.Background(), thread.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("disabled chat should not store messages, got %#v", messages)
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

func enableProviderChat(t *testing.T, store repository.Storage, providerType string, providerChatID string) *coremodel.Chat {
	t.Helper()

	chat, err := store.Chats().EnsureProviderChat(context.Background(), providerType, providerChatID)
	if err != nil {
		t.Fatalf("ensure chat: %v", err)
	}
	chat.Enabled = true
	if err := store.Chats().Save(context.Background(), chat); err != nil {
		t.Fatalf("enable chat: %v", err)
	}
	return chat
}

type fakeAgent struct {
	typ   string
	reply string
	calls int
}

var _ component.Agent = (*fakeAgent)(nil)

func (a *fakeAgent) Type() string {
	if a.typ != "" {
		return a.typ
	}
	return "fake-agent"
}

func (a *fakeAgent) HandleMessage(_ context.Context, message coremodel.ThreadMessage) (*coremodel.ThreadMessage, error) {
	a.calls++
	return &coremodel.ThreadMessage{
		Kind:       coremodel.MessageKindAgent,
		SourceType: a.Type(),
		ActorID:    a.Type(),
		ActorLabel: "Fake Agent",
		Text:       a.reply,
	}, nil
}

type fakeRelay struct {
	typ  string
	sent []coremodel.ThreadMessage
}

var _ component.OutboundRelay = (*fakeRelay)(nil)

func (r *fakeRelay) Type() string {
	if r.typ != "" {
		return r.typ
	}
	return "fake-relay"
}

func (r *fakeRelay) SendMessage(_ context.Context, message coremodel.ThreadMessage) error {
	r.sent = append(r.sent, message)
	return nil
}
