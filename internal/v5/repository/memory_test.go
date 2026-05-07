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
		ThreadID:         threadID,
		ComponentID:      componentID,
		ProviderThreadID: "provider-thread-1",
		StateJSON:        `{"keep_running":true}`,
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
	if got, want := loaded.ProviderThreadID, "provider-thread-1"; got != want {
		t.Fatalf("ProviderThreadID = %q, want %q", got, want)
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
