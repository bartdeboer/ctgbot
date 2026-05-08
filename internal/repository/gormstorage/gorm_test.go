package gormstorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

func TestArtifactsStoreContentOnDiskOnly(t *testing.T) {
	ctx := context.Background()
	store := newTestStoreWithArtifactDir(t)

	messageID := modeluuid.New()
	artifact := &coremodel.Artifact{
		ChatID:      modeluuid.New(),
		ThreadID:    modeluuid.New(),
		MessageID:   messageID,
		ComponentID: modeluuid.New(),
		Filename:    "stdin.txt",
		ContentType: "text/plain",
		Content:     []byte("hello from disk"),
	}
	if err := store.Artifacts().Append(ctx, artifact); err != nil {
		t.Fatalf("Artifacts().Append() error = %v", err)
	}
	if got, want := artifact.StorageKind, "file"; got != want {
		t.Fatalf("StorageKind = %q, want %q", got, want)
	}
	if artifact.StoragePath == "" {
		t.Fatal("StoragePath is empty")
	}
	if got, want := artifact.Size, int64(len("hello from disk")); got != want {
		t.Fatalf("Size = %d, want %d", got, want)
	}
	if artifact.SHA256 == "" {
		t.Fatal("SHA256 is empty")
	}
	if len(artifact.Content) != 0 {
		t.Fatalf("artifact kept %d bytes in memory after save, want 0", len(artifact.Content))
	}
	onDisk, err := os.ReadFile(filepath.Join(store.artifacts.artifactDir, artifact.StoragePath))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(onDisk), "hello from disk"; got != want {
		t.Fatalf("disk content = %q, want %q", got, want)
	}

	var raw struct {
		StorageKind string `gorm:"column:storage_kind"`
		StoragePath string `gorm:"column:storage_path"`
		Size        int64  `gorm:"column:size"`
		SHA256      string `gorm:"column:sha256"`
	}
	if err := store.db.WithContext(ctx).
		Table("artifacts").
		Select("storage_kind, storage_path, size, sha256").
		Where("id = ?", artifact.ID).
		Scan(&raw).Error; err != nil {
		t.Fatalf("raw artifact metadata load error = %v", err)
	}
	if raw.StorageKind != "file" || raw.StoragePath == "" || raw.Size == 0 || raw.SHA256 == "" {
		t.Fatalf("raw artifact metadata = %#v", raw)
	}

	loaded, err := store.Artifacts().ListByMessageID(ctx, messageID)
	if err != nil {
		t.Fatalf("Artifacts().ListByMessageID() error = %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loaded artifacts = %d, want 1", len(loaded))
	}
	if got, want := string(loaded[0].Content), "hello from disk"; got != want {
		t.Fatalf("loaded content = %q, want %q", got, want)
	}
}

func TestArtifactsAppendFailsWithoutArtifactDir(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	err := store.Artifacts().Append(ctx, &coremodel.Artifact{
		ChatID:      modeluuid.New(),
		ThreadID:    modeluuid.New(),
		MessageID:   modeluuid.New(),
		ComponentID: modeluuid.New(),
		Filename:    "stdin.txt",
		ContentType: "text/plain",
		Content:     []byte("hello"),
	})
	if err == nil {
		t.Fatal("Artifacts().Append() unexpectedly succeeded without artifact dir")
	}
	if !strings.Contains(err.Error(), "artifact storage directory is not configured") {
		t.Fatalf("Artifacts().Append() error = %v", err)
	}
}

func TestAutoMigrateDropsLegacyArtifactContentColumn(t *testing.T) {
	ctx := context.Background()
	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%s?mode=memory&cache=shared", name, modeluuid.New().String())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	type legacyArtifact struct {
		ID      modeluuid.UUID `gorm:"primaryKey"`
		Content []byte
	}
	if err := db.WithContext(ctx).AutoMigrate(&legacyArtifact{}); err != nil {
		t.Fatalf("legacy AutoMigrate() error = %v", err)
	}
	store := NewWithArtifactDir(db, filepath.Join(t.TempDir(), "artifacts"))
	if err := store.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if db.WithContext(ctx).Migrator().HasColumn("artifacts", "content") {
		t.Fatal("artifacts.content column still exists after migrate")
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

func newTestStoreWithArtifactDir(t *testing.T) *GORMStorage {
	t.Helper()
	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%s?mode=memory&cache=shared", name, modeluuid.New().String())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	store := NewWithArtifactDir(db, filepath.Join(t.TempDir(), "artifacts"))
	if err := store.AutoMigrate(context.Background()); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	return store
}
