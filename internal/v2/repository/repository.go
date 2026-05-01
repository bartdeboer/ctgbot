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
	Components() ComponentRepository
	ComponentProfiles() ComponentProfileRepository
	ChatComponents() ChatComponentRepository
	ThreadComponentStates() ThreadComponentStateRepository
	Messages() MessageRepository
	Artifacts() ArtifactRepository
}

type ChatRepository interface {
	Save(ctx context.Context, chat *coremodel.Chat) error
	GetByID(ctx context.Context, chatID modeluuid.UUID) (*coremodel.Chat, error)
	EnsureProviderChat(ctx context.Context, providerType string, providerChatID string) (*coremodel.Chat, error)
	ListDisabled(ctx context.Context) ([]coremodel.Chat, error)
}

type ThreadRepository interface {
	Save(ctx context.Context, thread *coremodel.Thread) error
	GetByID(ctx context.Context, threadID modeluuid.UUID) (*coremodel.Thread, error)
	EnsureProviderThread(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*coremodel.Thread, error)
	ListByChatID(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.Thread, error)
}

type ComponentRepository interface {
	Save(ctx context.Context, component *coremodel.Component) error
	GetByType(ctx context.Context, componentType string) (*coremodel.Component, error)
}

type ComponentProfileRepository interface {
	Save(ctx context.Context, profile *coremodel.ComponentProfile) error
	Get(ctx context.Context, componentType string, profileName string) (*coremodel.ComponentProfile, error)
}

type ChatComponentRepository interface {
	Save(ctx context.Context, binding *coremodel.ChatComponent) error
	ListByChatID(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.ChatComponent, error)
	ListEnabledByChatID(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.ChatComponent, error)
}

type ThreadComponentStateRepository interface {
	Save(ctx context.Context, state *coremodel.ThreadComponentState) error
	Get(ctx context.Context, threadID modeluuid.UUID, componentType string, profileName string) (*coremodel.ThreadComponentState, error)
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
