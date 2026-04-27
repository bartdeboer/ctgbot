package appstate

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/dbmodel"
	"github.com/bartdeboer/ctgbot/internal/dbstorage"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
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

type fakeStorage struct{}

func (fakeStorage) AutoMigrate(ctx context.Context) error { return nil }
func (fakeStorage) Threads() dbstorage.ThreadStorage      { return fakeThreadStorage{} }

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
