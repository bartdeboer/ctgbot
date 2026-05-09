package repository

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type Storage interface {
	Transaction(ctx context.Context, fn func(Storage) error) error
	Chats() ChatRepository
	Threads() ThreadRepository
	Components() ComponentRepository
	ChatComponents() ChatComponentRepository
	InboundDrops() InboundDropRepository
	ThreadComponentMappings() ThreadComponentMappingRepository
	ThreadComponentStates() ThreadComponentStateRepository
	Messages() MessageRepository
	Artifacts() ArtifactRepository
}

type ChatRepository interface {
	Save(ctx context.Context, chat *coremodel.Chat) error
	GetByID(ctx context.Context, chatID modeluuid.UUID) (*coremodel.Chat, error)
	List(ctx context.Context) ([]coremodel.Chat, error)
}

type ShortIDSource interface {
	ListIDs(ctx context.Context) ([]modeluuid.UUID, error)
}

type ThreadRepository interface {
	ShortIDSource
	Save(ctx context.Context, thread *coremodel.Thread) error
	GetByID(ctx context.Context, threadID modeluuid.UUID) (*coremodel.Thread, error)
	ListByChatID(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.Thread, error)
}

type ComponentRepository interface {
	Save(ctx context.Context, component *coremodel.Component) error
	GetByID(ctx context.Context, componentID modeluuid.UUID) (*coremodel.Component, error)
	GetDefaultByType(ctx context.Context, componentType string) (*coremodel.Component, error)
	GetByTypeAndName(ctx context.Context, componentType string, name string) (*coremodel.Component, error)
	ListEnabled(ctx context.Context) ([]coremodel.Component, error)
}

type ChatComponentRepository interface {
	Save(ctx context.Context, binding *coremodel.ChatComponent) error
	GetByChatComponentRole(ctx context.Context, chatID modeluuid.UUID, componentID modeluuid.UUID, role coremodel.ChatComponentRole) (*coremodel.ChatComponent, error)
	ListEnabledByChatID(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.ChatComponent, error)
	FindByComponentRoleAndExternalChatID(ctx context.Context, componentID modeluuid.UUID, role coremodel.ChatComponentRole, externalChatID string) (*coremodel.ChatComponent, error)
}

type InboundDropRepository interface {
	Save(ctx context.Context, drop *coremodel.InboundDrop) error
	GetByComponentAndExternalChatID(ctx context.Context, componentID modeluuid.UUID, externalChatID string) (*coremodel.InboundDrop, error)
	List(ctx context.Context) ([]coremodel.InboundDrop, error)
	DeleteByComponentAndExternalChatID(ctx context.Context, componentID modeluuid.UUID, externalChatID string) error
}

type ThreadComponentMappingRepository interface {
	Save(ctx context.Context, mapping *coremodel.ThreadComponentMapping) error
	GetByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) (*coremodel.ThreadComponentMapping, error)
	FindByChatComponentAndThreadID(ctx context.Context, chatID modeluuid.UUID, componentID modeluuid.UUID, componentThreadID string) (*coremodel.ThreadComponentMapping, error)
	DeleteByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) error
}

type ThreadComponentStateRepository interface {
	Save(ctx context.Context, state *coremodel.ThreadComponentState) error
	GetByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) (*coremodel.ThreadComponentState, error)
	DeleteByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) error
}

type MessageRepository interface {
	Append(ctx context.Context, message *coremodel.ThreadMessage) error
	ListByThreadID(ctx context.Context, threadID modeluuid.UUID) ([]coremodel.ThreadMessage, error)
}

type ArtifactRepository interface {
	Append(ctx context.Context, artifact *coremodel.Artifact) error
	ListByMessageID(ctx context.Context, messageID modeluuid.UUID) ([]coremodel.Artifact, error)
}
