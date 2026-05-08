package gormstorage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTransactionRollsBackOnError(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	chat := &coremodel.Chat{Label: "before", Enabled: true}
	if err := store.Chats().Save(ctx, chat); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	wantErr := errors.New("boom")
	err := store.Transaction(ctx, func(tx repository.Storage) error {
		copy := *chat
		copy.Label = "after"
		if err := tx.Chats().Save(ctx, &copy); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Transaction() error = %v, want %v", err, wantErr)
	}

	stored, err := store.Chats().GetByID(ctx, chat.ID)
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

func TestTransactionCommitsOnSuccess(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	chat := &coremodel.Chat{Label: "before", Enabled: true}
	if err := store.Chats().Save(ctx, chat); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	err := store.Transaction(ctx, func(tx repository.Storage) error {
		copy := *chat
		copy.Label = "after"
		return tx.Chats().Save(ctx, &copy)
	})
	if err != nil {
		t.Fatalf("Transaction() error = %v", err)
	}

	stored, err := store.Chats().GetByID(ctx, chat.ID)
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

func TestAutoMigrateMigratesLegacyCodexThreadState(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for _, stmt := range []string{
		`ALTER TABLE threads ADD COLUMN keep_running numeric DEFAULT 0`,
		`ALTER TABLE threads ADD COLUMN codex_model text DEFAULT ''`,
		`ALTER TABLE threads ADD COLUMN codex_reasoning_effort text DEFAULT ''`,
	} {
		if err := store.db.WithContext(ctx).Exec(stmt).Error; err != nil {
			t.Fatalf("Exec(%q) error = %v", stmt, err)
		}
	}

	chat := &coremodel.Chat{ID: modeluuid.New(), Label: "team", Enabled: true}
	if err := store.Chats().Save(ctx, chat); err != nil {
		t.Fatalf("Chats().Save() error = %v", err)
	}
	thread := &coremodel.Thread{ID: modeluuid.New(), ChatID: chat.ID}
	if err := store.Threads().Save(ctx, thread); err != nil {
		t.Fatalf("Threads().Save() error = %v", err)
	}
	componentIDs := []modeluuid.UUID{modeluuid.New(), modeluuid.New()}
	for idx, componentID := range componentIDs {
		componentRow := &coremodel.Component{
			ID:      componentID,
			Type:    "codex",
			Name:    fmt.Sprintf("codex-%d", idx+1),
			Runtime: "docker",
			Enabled: true,
		}
		if err := store.Components().Save(ctx, componentRow); err != nil {
			t.Fatalf("Components().Save() error = %v", err)
		}
		if err := store.ChatComponents().Save(ctx, &coremodel.ChatComponent{
			ChatID:      chat.ID,
			ComponentID: componentID,
			Role:        coremodel.ChatComponentRoleAgent,
			Enabled:     true,
		}); err != nil {
			t.Fatalf("ChatComponents().Save() error = %v", err)
		}
	}
	otherComponent := &coremodel.Component{
		ID:      modeluuid.New(),
		Type:    "llamacpp",
		Name:    "qwen",
		Runtime: "backend",
		Enabled: true,
	}
	if err := store.Components().Save(ctx, otherComponent); err != nil {
		t.Fatalf("Components().Save() other error = %v", err)
	}
	if err := store.ChatComponents().Save(ctx, &coremodel.ChatComponent{
		ChatID:      chat.ID,
		ComponentID: otherComponent.ID,
		Role:        coremodel.ChatComponentRoleAgent,
		Enabled:     true,
	}); err != nil {
		t.Fatalf("ChatComponents().Save() other error = %v", err)
	}

	if err := store.db.WithContext(ctx).Exec(
		`UPDATE threads SET keep_running = ?, codex_model = ?, codex_reasoning_effort = ? WHERE id = ?`,
		true, "gpt-5.5", "high", thread.ID,
	).Error; err != nil {
		t.Fatalf("seed legacy thread state error = %v", err)
	}

	if err := store.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate() rerun error = %v", err)
	}

	for _, componentID := range componentIDs {
		row, err := store.ThreadComponentStates().GetByThreadAndComponent(ctx, thread.ID, componentID)
		if err != nil {
			t.Fatalf("GetByThreadAndComponent(%s) error = %v", componentID, err)
		}
		if row == nil {
			t.Fatalf("expected migrated thread component state for component %s", componentID)
		}
		state, err := decodeLegacyCodexThreadState(row)
		if err != nil {
			t.Fatalf("decodeLegacyCodexThreadState(%s) error = %v", componentID, err)
		}
		if state.KeepRunning == nil || !*state.KeepRunning {
			t.Fatalf("keep_running for %s = %#v, want true", componentID, state.KeepRunning)
		}
		if got, want := state.Model, "gpt-5.5"; got != want {
			t.Fatalf("model for %s = %q, want %q", componentID, got, want)
		}
		if got, want := state.ReasoningEffort, "high"; got != want {
			t.Fatalf("reasoning effort for %s = %q, want %q", componentID, got, want)
		}
	}

	otherRow, err := store.ThreadComponentStates().GetByThreadAndComponent(ctx, thread.ID, otherComponent.ID)
	if err != nil {
		t.Fatalf("GetByThreadAndComponent(other) error = %v", err)
	}
	if otherRow != nil {
		t.Fatalf("expected no migrated state for non-codex component, got %#v", otherRow)
	}

	var cleared struct {
		KeepRunning          bool   `gorm:"column:keep_running"`
		CodexModel           string `gorm:"column:codex_model"`
		CodexReasoningEffort string `gorm:"column:codex_reasoning_effort"`
	}
	if err := store.db.WithContext(ctx).Raw(
		`SELECT keep_running, codex_model, codex_reasoning_effort FROM threads WHERE id = ?`,
		thread.ID,
	).Scan(&cleared).Error; err != nil {
		t.Fatalf("Raw() cleared legacy thread state error = %v", err)
	}
	if cleared.KeepRunning || cleared.CodexModel != "" || cleared.CodexReasoningEffort != "" {
		t.Fatalf("legacy thread state not cleared: %#v", cleared)
	}
}

func newTestStore(t *testing.T) *GORMStorage {
	t.Helper()
	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%s?mode=memory&cache=shared", name, modeluuid.New().String())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	store := New(db)
	if err := store.AutoMigrate(context.Background()); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	return store
}
