package dbstorage

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/dbmodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type Storage interface {
	AutoMigrate(ctx context.Context) error
	Threads() ThreadStorage
	TelegramUpdates() TelegramUpdateStorage
}

type ThreadStorage interface {
	GetByID(ctx context.Context, threadID modeluuid.UUID) (*dbmodel.Thread, error)
	GetByProviderThreadID(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*dbmodel.Thread, error)
	EnsureProviderThread(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*dbmodel.Thread, error)
	Save(ctx context.Context, thread *dbmodel.Thread) error

	WorkspaceHost(ctx context.Context, threadID modeluuid.UUID) (string, error)
	SetWorkspaceHost(ctx context.Context, threadID modeluuid.UUID, value string) error
	AgentThreadID(ctx context.Context, threadID modeluuid.UUID) (string, error)
	SetAgentThreadID(ctx context.Context, threadID modeluuid.UUID, value string) error
	KeepRunning(ctx context.Context, threadID modeluuid.UUID) (bool, error)
	SetKeepRunning(ctx context.Context, threadID modeluuid.UUID, value bool) error
}

type TelegramUpdateStorage interface {
	Create(ctx context.Context, event *dbmodel.TelegramUpdate) error
	Save(ctx context.Context, event *dbmodel.TelegramUpdate) error
}
