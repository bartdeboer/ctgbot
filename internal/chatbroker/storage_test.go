package chatbroker

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
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

	chatID := modeluuid.New()

	thread, err := store.EnsureThread(ctx, chatID, "7")
	if err != nil {
		t.Fatalf("ensure thread: %v", err)
	}
	if thread.ID.IsNull() {
		t.Fatalf("expected thread UUID to be set")
	}

	thread.Active = true
	thread.AgentProviderType = "codex"
	thread.RuntimeName = "ctgbot-1-7"
	if err := store.SaveThread(ctx, thread); err != nil {
		t.Fatalf("save thread: %v", err)
	}

	got, err := store.FindThread(ctx, chatID, "7")
	if err != nil {
		t.Fatalf("find thread: %v", err)
	}
	if got == nil {
		t.Fatalf("expected thread")
	}
	if !got.Active {
		t.Fatalf("expected thread to be active")
	}
	if got.RuntimeName != "ctgbot-1-7" {
		t.Fatalf("got container name %q", got.RuntimeName)
	}

	byID, err := store.FindThreadByID(ctx, thread.ID)
	if err != nil {
		t.Fatalf("find thread by id: %v", err)
	}
	if byID == nil {
		t.Fatalf("expected thread by id")
	}
	if byID.ProviderThreadID != "7" {
		t.Fatalf("byID.ProviderThreadID = %q", byID.ProviderThreadID)
	}
}
