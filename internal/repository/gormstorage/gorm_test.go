package gormstorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestChatsShortIDs(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	first := fixedUUID(1)
	second := fixedUUID(2)

	for _, chat := range []*coremodel.Chat{
		{ID: first, Label: "first", Enabled: true},
		{ID: second, Label: "second", Enabled: true},
	} {
		if err := store.Chats().Save(ctx, chat); err != nil {
			t.Fatalf("Chats().Save() error = %v", err)
		}
	}

	ids, err := store.Chats().ListIDs(ctx)
	if err != nil {
		t.Fatalf("Chats().ListIDs() error = %v", err)
	}
	resolver := repository.NewShortIDResolver(ids)

	shortID, err := resolver.ShortIDFor(first, 1)
	if err != nil {
		t.Fatalf("ShortIDFor() error = %v", err)
	}
	if !strings.HasSuffix(first.String(), shortID) {
		t.Fatalf("short ID %q is not a suffix of %s", shortID, first)
	}
	resolved, err := resolver.Resolve(shortID)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved != first {
		t.Fatalf("resolved = %s, want %s", resolved, first)
	}
}

func TestThreadsShortIDs(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	chatID := modeluuid.New()
	first := fixedUUID(1)
	second := fixedUUID(2)

	for _, thread := range []*coremodel.Thread{
		{ID: first, ChatID: chatID, Label: "first"},
		{ID: second, ChatID: chatID, Label: "second"},
	} {
		if err := store.Threads().Save(ctx, thread); err != nil {
			t.Fatalf("Threads().Save() error = %v", err)
		}
	}

	ids, err := store.Threads().ListIDs(ctx)
	if err != nil {
		t.Fatalf("Threads().ListIDs() error = %v", err)
	}
	resolver := repository.NewShortIDResolver(ids)

	shortID, err := resolver.ShortIDFor(first, 1)
	if err != nil {
		t.Fatalf("ShortIDFor() error = %v", err)
	}
	if !strings.HasSuffix(first.String(), shortID) {
		t.Fatalf("short ID %q is not a suffix of %s", shortID, first)
	}
	if shortID == first.String() {
		t.Fatalf("short ID = full ID %q, want shortest unique suffix", shortID)
	}

	resolved, err := resolver.Resolve(shortID)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved != first {
		t.Fatalf("resolved = %s, want %s", resolved, first)
	}

	_, err = resolver.Resolve("0")
	var ambiguous *repository.ShortIDAmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("Resolve(\"0\") error = %v, want ambiguous", err)
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

func TestTransactionPreservesArtifactDir(t *testing.T) {
	ctx := context.Background()
	store := newTestStoreWithArtifactDir(t)
	messageID := modeluuid.New()

	err := store.Transaction(ctx, func(tx repository.Storage) error {
		return tx.Artifacts().Append(ctx, &coremodel.Artifact{
			ChatID:      modeluuid.New(),
			ThreadID:    modeluuid.New(),
			MessageID:   messageID,
			ComponentID: modeluuid.New(),
			Filename:    "stdin.txt",
			ContentType: "text/plain",
			Content:     []byte("hello from transaction"),
		})
	})
	if err != nil {
		t.Fatalf("Transaction(artifact append) error = %v", err)
	}

	artifacts, err := store.Artifacts().ListByMessageID(ctx, messageID)
	if err != nil {
		t.Fatalf("Artifacts().ListByMessageID() error = %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("len(artifacts) = %d, want 1", len(artifacts))
	}
	if got, want := string(artifacts[0].Content), "hello from transaction"; got != want {
		t.Fatalf("artifact content = %q, want %q", got, want)
	}
}

func TestInboundDropsSaveListGetDelete(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	componentID := modeluuid.New()
	now := time.Now()

	drop := &coremodel.InboundDrop{
		ComponentID:      componentID,
		ExternalChatID:   "chat-1",
		ExternalThreadID: "thread-9",
		ChatLabel:        "New chat",
		ActorID:          "bart",
		ActorLabel:       "Bart",
		LastTextPreview:  "hello",
		MessageCount:     2,
		FirstSeenAt:      now.Add(-time.Minute),
		LastSeenAt:       now,
	}
	if err := store.InboundDrops().Save(ctx, drop); err != nil {
		t.Fatalf("InboundDrops().Save() error = %v", err)
	}
	if drop.ID.IsNull() {
		t.Fatal("InboundDrops().Save() did not assign ID")
	}

	loaded, err := store.InboundDrops().GetByComponentAndExternalChatID(ctx, componentID, "chat-1")
	if err != nil {
		t.Fatalf("InboundDrops().GetByComponentAndExternalChatID() error = %v", err)
	}
	if loaded == nil {
		t.Fatal("InboundDrops().GetByComponentAndExternalChatID() = nil, want row")
	}
	if got, want := loaded.MessageCount, int64(2); got != want {
		t.Fatalf("MessageCount = %d, want %d", got, want)
	}

	list, err := store.InboundDrops().List(ctx)
	if err != nil {
		t.Fatalf("InboundDrops().List() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("InboundDrops().List() len = %d, want 1", len(list))
	}

	if err := store.InboundDrops().DeleteByComponentAndExternalChatID(ctx, componentID, "chat-1"); err != nil {
		t.Fatalf("InboundDrops().DeleteByComponentAndExternalChatID() error = %v", err)
	}
	loaded, err = store.InboundDrops().GetByComponentAndExternalChatID(ctx, componentID, "chat-1")
	if err != nil {
		t.Fatalf("InboundDrops().GetByComponentAndExternalChatID() after delete error = %v", err)
	}
	if loaded != nil {
		t.Fatalf("InboundDrops().GetByComponentAndExternalChatID() after delete = %#v, want nil", loaded)
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

func fixedUUID(last byte) modeluuid.UUID {
	var id modeluuid.UUID
	id[6] = last
	return id
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
