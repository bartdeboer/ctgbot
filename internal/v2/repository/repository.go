// Package repository defines storage contracts for the v2 conversation model.
package repository

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

type Storage interface {
	AutoMigrate(ctx context.Context) error
	Chats() ChatRepository
	Threads() ThreadRepository
	Messages() MessageRepository
	Artifacts() ArtifactRepository
}

type ChatRepository interface {
	Save(ctx context.Context, chat *coremodel.Chat) error
	GetByID(ctx context.Context, chatID modeluuid.UUID) (*coremodel.Chat, error)
}

type ThreadRepository interface {
	Save(ctx context.Context, thread *coremodel.Thread) error
	GetByID(ctx context.Context, threadID modeluuid.UUID) (*coremodel.Thread, error)
	ListByChatID(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.Thread, error)
}

type MessageRepository interface {
	Append(ctx context.Context, message *coremodel.ThreadMessage) error
	GetByID(ctx context.Context, messageID modeluuid.UUID) (*coremodel.ThreadMessage, error)
	ListByThreadID(ctx context.Context, threadID modeluuid.UUID) ([]coremodel.ThreadMessage, error)
}

type ArtifactRepository interface {
	Append(ctx context.Context, artifact *coremodel.Artifact) error
	ListByMessageID(ctx context.Context, messageID modeluuid.UUID) ([]coremodel.Artifact, error)
}
