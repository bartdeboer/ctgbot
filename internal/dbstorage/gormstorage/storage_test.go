package gormstorage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestStorageEnsuresProviderThreadAndPersistsThread(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ctgbot.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	store := New(db)
	ctx := context.Background()

	if err := store.AutoMigrate(ctx); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	chatID := modeluuid.New()
	thread, err := store.Threads().EnsureProviderThread(ctx, chatID, "7")
	if err != nil {
		t.Fatalf("ensure thread: %v", err)
	}
	if thread.ID.IsNull() {
		t.Fatalf("expected thread UUID to be set")
	}

	thread.Active = true
	thread.AgentProviderType = "codex"
	thread.RuntimeName = "ctgbot-1-7"
	if err := store.Threads().Save(ctx, thread); err != nil {
		t.Fatalf("save thread: %v", err)
	}

	got, err := store.Threads().GetByProviderThreadID(ctx, chatID, "7")
	if err != nil {
		t.Fatalf("find thread: %v", err)
	}
	if got == nil || !got.Active || got.RuntimeName != "ctgbot-1-7" {
		t.Fatalf("unexpected thread: %#v", got)
	}

	byID, err := store.Threads().GetByID(ctx, thread.ID)
	if err != nil {
		t.Fatalf("find thread by id: %v", err)
	}
	if byID == nil || byID.ProviderThreadID != "7" {
		t.Fatalf("unexpected thread by id: %#v", byID)
	}
}

func TestThreadFieldAccessors(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ctgbot.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	store := New(db)
	ctx := context.Background()

	if err := store.AutoMigrate(ctx); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	thread, err := store.Threads().EnsureProviderThread(ctx, modeluuid.New(), "9")
	if err != nil {
		t.Fatalf("ensure thread: %v", err)
	}

	if err := store.Threads().SetWorkspaceHost(ctx, thread.ID, " /workspace "); err != nil {
		t.Fatalf("set workspace: %v", err)
	}
	if err := store.Threads().SetAgentThreadID(ctx, thread.ID, " provider-thread "); err != nil {
		t.Fatalf("set agent thread id: %v", err)
	}

	workspace, err := store.Threads().WorkspaceHost(ctx, thread.ID)
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}
	agentThreadID, err := store.Threads().AgentThreadID(ctx, thread.ID)
	if err != nil {
		t.Fatalf("agent thread id: %v", err)
	}
	if workspace != "/workspace" || agentThreadID != "provider-thread" {
		t.Fatalf("unexpected values workspace=%q agent_thread_id=%q", workspace, agentThreadID)
	}
}
