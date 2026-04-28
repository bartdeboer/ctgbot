package gormstorage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/dbmodel"
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

func TestStoragePersistsTelegramUpdates(t *testing.T) {
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

	event := &dbmodel.TelegramUpdate{
		ChatID:    42,
		MessageID: 99,
		Text:      "hello",
		Username:  "bart",
	}
	if err := store.TelegramUpdates().Create(ctx, event); err != nil {
		t.Fatalf("create telegram update: %v", err)
	}
	if event.ID == 0 {
		t.Fatal("expected telegram update id")
	}

	event.ResponseText = "reply"
	if err := store.TelegramUpdates().Save(ctx, event); err != nil {
		t.Fatalf("save telegram update: %v", err)
	}

	var got dbmodel.TelegramUpdate
	if err := db.WithContext(ctx).First(&got, event.ID).Error; err != nil {
		t.Fatalf("load telegram update: %v", err)
	}
	if got.ResponseText != "reply" || got.UserLabel() != "@bart" {
		t.Fatalf("unexpected telegram update: %#v", got)
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
	if err := store.Threads().SetCodexModel(ctx, thread.ID, " gpt-test "); err != nil {
		t.Fatalf("set codex model: %v", err)
	}
	if err := store.Threads().SetCodexReasoningEffort(ctx, thread.ID, " high "); err != nil {
		t.Fatalf("set codex reasoning effort: %v", err)
	}
	if err := store.Threads().SetKeepRunning(ctx, thread.ID, true); err != nil {
		t.Fatalf("set keep running: %v", err)
	}

	workspace, err := store.Threads().WorkspaceHost(ctx, thread.ID)
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}
	agentThreadID, err := store.Threads().AgentThreadID(ctx, thread.ID)
	if err != nil {
		t.Fatalf("agent thread id: %v", err)
	}
	codexModel, err := store.Threads().CodexModel(ctx, thread.ID)
	if err != nil {
		t.Fatalf("codex model: %v", err)
	}
	reasoningEffort, err := store.Threads().CodexReasoningEffort(ctx, thread.ID)
	if err != nil {
		t.Fatalf("codex reasoning effort: %v", err)
	}
	keepRunning, err := store.Threads().KeepRunning(ctx, thread.ID)
	if err != nil {
		t.Fatalf("keep running: %v", err)
	}
	if workspace != "/workspace" || agentThreadID != "provider-thread" || codexModel != "gpt-test" || reasoningEffort != "high" || !keepRunning {
		t.Fatalf("unexpected values workspace=%q agent_thread_id=%q codex_model=%q reasoning_effort=%q keep_running=%t", workspace, agentThreadID, codexModel, reasoningEffort, keepRunning)
	}
}
