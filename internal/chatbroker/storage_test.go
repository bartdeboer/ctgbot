package chatbroker

import (
	"context"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSessionStorageEnsuresChatThreadAndPersistsThreadState(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ctgbot.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	store := NewSessionStorage(db)
	ctx := context.Background()

	if err := store.AutoMigrate(ctx); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	chat, err := store.EnsureChat(ctx, "telegram", "-100123", "Test Chat")
	if err != nil {
		t.Fatalf("ensure chat: %v", err)
	}
	if chat.ID.IsNull() {
		t.Fatalf("expected chat UUID to be set")
	}

	thread, err := store.EnsureThread(ctx, chat.ID, "7")
	if err != nil {
		t.Fatalf("ensure thread: %v", err)
	}
	if thread.ID.IsNull() {
		t.Fatalf("expected thread UUID to be set")
	}

	thread.Active = true
	thread.ProviderType = "codex"
	thread.ContainerName = "ctgbot-1-7"
	if err := store.SaveThread(ctx, thread); err != nil {
		t.Fatalf("save thread: %v", err)
	}

	got, err := store.FindThread(ctx, chat.ID, "7")
	if err != nil {
		t.Fatalf("find thread: %v", err)
	}
	if got == nil {
		t.Fatalf("expected thread")
	}
	if !got.Active {
		t.Fatalf("expected thread to be active")
	}
	if got.ContainerName != "ctgbot-1-7" {
		t.Fatalf("got container name %q", got.ContainerName)
	}
}
