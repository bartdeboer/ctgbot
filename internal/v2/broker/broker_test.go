package broker

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v2/component"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v2/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBrokerRoutesInboundEventIntoThreadMessage(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	chatID := modeluuid.New()
	threadID := modeluuid.New()

	broker := New(store, component.NewRegistry())
	message, err := broker.RouteInboundEvent(context.Background(), component.InboundEvent{
		SourceType: "telegram",
		EventType:  "message.received",
		ExternalID: "telegram:1:2:3",
		ChatID:     chatID,
		ThreadID:   threadID,
		Actor:      component.Actor{ID: "13145044", Label: "@bartdeboer"},
		Text:       "hello",
	})
	if err != nil {
		t.Fatalf("route inbound event: %v", err)
	}
	if message.Direction != coremodel.DirectionInbound || message.Kind != coremodel.MessageKindUser {
		t.Fatalf("unexpected message shape: %#v", message)
	}

	messages, err := store.Messages().ListByThreadID(context.Background(), threadID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 || messages[0].Text != "hello" || messages[0].SourceType != "telegram" {
		t.Fatalf("unexpected messages: %#v", messages)
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
