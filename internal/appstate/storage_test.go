package appstate

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/dbmodel"
	"github.com/bartdeboer/ctgbot/internal/dbstorage"
	"github.com/bartdeboer/ctgbot/internal/dbstorage/gormstorage"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestConfigStorage(t *testing.T) {
	cfg, _ := newTestConfig(t)
	storage := fakeStorage{}

	if cfg.Storage() != nil {
		t.Fatal("expected no storage by default")
	}
	cfg.SetStorage(storage)
	if cfg.Storage() != storage {
		t.Fatal("expected configured storage")
	}
}

func TestThreadKeepRunningUsesConfiguredStorage(t *testing.T) {
	cfg, _ := newTestConfig(t)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ctgbot.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	storage := gormstorage.New(db)
	cfg.SetStorage(storage)
	ctx := context.Background()

	if err := storage.AutoMigrate(ctx); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	chatID := modeluuid.New()
	thread, err := storage.Threads().EnsureProviderThread(ctx, chatID, "7")
	if err != nil {
		t.Fatalf("ensure thread: %v", err)
	}

	if err := cfg.Thread(chatID, thread.ID).SetKeepRunning(ctx, true); err != nil {
		t.Fatalf("set keep running: %v", err)
	}
	keepRunning, err := cfg.Thread(chatID, thread.ID).KeepRunning(ctx)
	if err != nil {
		t.Fatalf("keep running: %v", err)
	}
	if !keepRunning {
		t.Fatal("expected keep running")
	}
}

type fakeStorage struct{}

func (fakeStorage) AutoMigrate(ctx context.Context) error { return nil }
func (fakeStorage) Threads() dbstorage.ThreadStorage      { return fakeThreadStorage{} }
func (fakeStorage) TelegramUpdates() dbstorage.TelegramUpdateStorage {
	return fakeTelegramUpdateStorage{}
}

type fakeThreadStorage struct{}

func (fakeThreadStorage) GetByID(ctx context.Context, threadID modeluuid.UUID) (*dbmodel.Thread, error) {
	return nil, nil
}

func (fakeThreadStorage) GetByProviderThreadID(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*dbmodel.Thread, error) {
	return nil, nil
}

func (fakeThreadStorage) EnsureProviderThread(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*dbmodel.Thread, error) {
	return nil, nil
}

func (fakeThreadStorage) Save(ctx context.Context, thread *dbmodel.Thread) error {
	return nil
}

func (fakeThreadStorage) WorkspaceHost(ctx context.Context, threadID modeluuid.UUID) (string, error) {
	return "", nil
}

func (fakeThreadStorage) SetWorkspaceHost(ctx context.Context, threadID modeluuid.UUID, value string) error {
	return nil
}

func (fakeThreadStorage) AgentThreadID(ctx context.Context, threadID modeluuid.UUID) (string, error) {
	return "", nil
}

func (fakeThreadStorage) SetAgentThreadID(ctx context.Context, threadID modeluuid.UUID, value string) error {
	return nil
}

func (fakeThreadStorage) KeepRunning(ctx context.Context, threadID modeluuid.UUID) (bool, error) {
	return false, nil
}

func (fakeThreadStorage) SetKeepRunning(ctx context.Context, threadID modeluuid.UUID, value bool) error {
	return nil
}

type fakeTelegramUpdateStorage struct{}

func (fakeTelegramUpdateStorage) Create(ctx context.Context, event *dbmodel.TelegramUpdate) error {
	return nil
}

func (fakeTelegramUpdateStorage) Save(ctx context.Context, event *dbmodel.TelegramUpdate) error {
	return nil
}
