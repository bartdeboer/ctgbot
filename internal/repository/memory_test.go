package repository

import (
	"context"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func TestMemoryThreadComponentStatesSaveGetDelete(t *testing.T) {
	ctx := context.Background()
	storage := NewMemory()
	threadID := modeluuid.New()
	componentID := modeluuid.New()

	row := &coremodel.ThreadComponentState{
		ThreadID:    threadID,
		ComponentID: componentID,
		StateJSON:   `{"keep_running":true}`,
	}
	if err := storage.ThreadComponentStates().Save(ctx, row); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if row.ID.IsNull() {
		t.Fatal("Save() did not assign ID")
	}

	loaded, err := storage.ThreadComponentStates().GetByThreadAndComponent(ctx, threadID, componentID)
	if err != nil {
		t.Fatalf("GetByThreadAndComponent() error = %v", err)
	}
	if loaded == nil {
		t.Fatal("GetByThreadAndComponent() = nil, want row")
	}
	if got, want := loaded.StateJSON, `{"keep_running":true}`; got != want {
		t.Fatalf("StateJSON = %q, want %q", got, want)
	}

	if err := storage.ThreadComponentStates().DeleteByThreadAndComponent(ctx, threadID, componentID); err != nil {
		t.Fatalf("DeleteByThreadAndComponent() error = %v", err)
	}
	loaded, err = storage.ThreadComponentStates().GetByThreadAndComponent(ctx, threadID, componentID)
	if err != nil {
		t.Fatalf("GetByThreadAndComponent() after delete error = %v", err)
	}
	if loaded != nil {
		t.Fatalf("GetByThreadAndComponent() after delete = %#v, want nil", loaded)
	}
}

func TestMemoryInboundDropsSaveListGetDelete(t *testing.T) {
	ctx := context.Background()
	storage := NewMemory()
	componentID := modeluuid.New()

	drop := &coremodel.InboundDrop{
		ComponentID:      componentID,
		ExternalChatID:   "chat-1",
		ExternalThreadID: "thread-9",
		ChatLabel:        "New chat",
		ActorID:          "bart",
		ActorLabel:       "Bart",
		LastTextPreview:  "hello",
		MessageCount:     1,
		FirstSeenAt:      time.Now().Add(-time.Minute),
		LastSeenAt:       time.Now(),
	}
	if err := storage.InboundDrops().Save(ctx, drop); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if drop.ID.IsNull() {
		t.Fatal("Save() did not assign ID")
	}

	loaded, err := storage.InboundDrops().GetByComponentAndExternalChatID(ctx, componentID, "chat-1")
	if err != nil {
		t.Fatalf("GetByComponentAndExternalChatID() error = %v", err)
	}
	if loaded == nil {
		t.Fatal("GetByComponentAndExternalChatID() = nil, want row")
	}
	if got, want := loaded.ChatLabel, "New chat"; got != want {
		t.Fatalf("ChatLabel = %q, want %q", got, want)
	}

	list, err := storage.InboundDrops().List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() len = %d, want 1", len(list))
	}

	if err := storage.InboundDrops().DeleteByComponentAndExternalChatID(ctx, componentID, "chat-1"); err != nil {
		t.Fatalf("DeleteByComponentAndExternalChatID() error = %v", err)
	}
	loaded, err = storage.InboundDrops().GetByComponentAndExternalChatID(ctx, componentID, "chat-1")
	if err != nil {
		t.Fatalf("GetByComponentAndExternalChatID() after delete error = %v", err)
	}
	if loaded != nil {
		t.Fatalf("GetByComponentAndExternalChatID() after delete = %#v, want nil", loaded)
	}
}

func TestMemoryTransactionRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	storage := NewMemory()

	chat := &coremodel.Chat{Label: "before", Enabled: true}
	if err := storage.Chats().Save(ctx, chat); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	wantErr := context.Canceled
	err := storage.Transaction(ctx, func(tx Storage) error {
		copy := *chat
		copy.Label = "after"
		if err := tx.Chats().Save(ctx, &copy); err != nil {
			return err
		}
		return wantErr
	})
	if err != wantErr {
		t.Fatalf("Transaction() error = %v, want %v", err, wantErr)
	}

	stored, err := storage.Chats().GetByID(ctx, chat.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if stored == nil {
		t.Fatal("expected stored chat")
	}
	if got, want := stored.Label, "before"; got != want {
		t.Fatalf("Label after rollback = %q, want %q", got, want)
	}
}

func TestMemoryTransactionCommitsOnSuccess(t *testing.T) {
	ctx := context.Background()
	storage := NewMemory()

	chat := &coremodel.Chat{Label: "before", Enabled: true}
	if err := storage.Chats().Save(ctx, chat); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if err := storage.Transaction(ctx, func(tx Storage) error {
		copy := *chat
		copy.Label = "after"
		return tx.Chats().Save(ctx, &copy)
	}); err != nil {
		t.Fatalf("Transaction() error = %v", err)
	}

	stored, err := storage.Chats().GetByID(ctx, chat.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if stored == nil {
		t.Fatal("expected stored chat")
	}
	if got, want := stored.Label, "after"; got != want {
		t.Fatalf("Label after commit = %q, want %q", got, want)
	}
}
