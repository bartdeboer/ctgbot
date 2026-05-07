package repository

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
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
