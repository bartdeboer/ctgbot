package repository

import (
	"context"
	"time"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type Storage interface {
	Transaction(ctx context.Context, fn func(Storage) error) error
	Chats() ChatRepository
	Threads() ThreadRepository
	Components() ComponentRepository
	ChatComponents() ChatComponentRepository
	InboundFilterBindings() InboundFilterBindingRepository
	InboundDrops() InboundDropRepository
	DroppedEvents() DroppedEventRepository
	AllowlistSenders() AllowlistSenderRepository
	ThreadComponentMappings() ThreadComponentMappingRepository
	ThreadComponentStates() ThreadComponentStateRepository
	Messages() MessageRepository
	Artifacts() ArtifactRepository
	ScheduledJobs() ScheduledJobRepository
	TrustedControllers() TrustedControllerRepository
}

type ChatRepository interface {
	ShortIDSource
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
	DeleteByID(ctx context.Context, componentID modeluuid.UUID) (bool, error)
}

type ChatComponentRepository interface {
	Save(ctx context.Context, binding *coremodel.ChatComponent) error
	GetByChatComponentRole(ctx context.Context, chatID modeluuid.UUID, componentID modeluuid.UUID, role coremodel.ChatComponentRole) (*coremodel.ChatComponent, error)
	ListEnabledByChatID(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.ChatComponent, error)
	ListByComponentID(ctx context.Context, componentID modeluuid.UUID) ([]coremodel.ChatComponent, error)
	FindByComponentRoleAndExternalChannelID(ctx context.Context, componentID modeluuid.UUID, role coremodel.ChatComponentRole, externalChannelID string) (*coremodel.ChatComponent, error)
	DeleteByComponentID(ctx context.Context, componentID modeluuid.UUID) (int64, error)
}

type InboundFilterBindingRepository interface {
	Save(ctx context.Context, binding *coremodel.InboundFilterBinding) error
	GetBySourceBindingAndFilter(ctx context.Context, sourceBindingID modeluuid.UUID, filterComponentID modeluuid.UUID) (*coremodel.InboundFilterBinding, error)
	ListEnabledBySourceBindingID(ctx context.Context, sourceBindingID modeluuid.UUID) ([]coremodel.InboundFilterBinding, error)
	DeleteByFilterComponentID(ctx context.Context, componentID modeluuid.UUID) (int64, error)
	DeleteBySourceBindingIDs(ctx context.Context, sourceBindingIDs []modeluuid.UUID) (int64, error)
}

type InboundDropRepository interface {
	Save(ctx context.Context, drop *coremodel.InboundDrop) error
	GetByComponentAndExternalChannelID(ctx context.Context, componentID modeluuid.UUID, externalChannelID string) (*coremodel.InboundDrop, error)
	List(ctx context.Context) ([]coremodel.InboundDrop, error)
	DeleteByComponentAndExternalChannelID(ctx context.Context, componentID modeluuid.UUID, externalChannelID string) error
}

type DroppedEventRepository interface {
	ShortIDSource
	Save(ctx context.Context, event *coremodel.DroppedEvent) error
	GetByID(ctx context.Context, eventID modeluuid.UUID) (*coremodel.DroppedEvent, error)
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
}

type AllowlistSenderRepository interface {
	Save(ctx context.Context, sender *coremodel.AllowlistSender) error
	GetBySourceBindingAndSenderKey(ctx context.Context, sourceBindingID modeluuid.UUID, senderKey string) (*coremodel.AllowlistSender, error)
	ListBySourceBindingID(ctx context.Context, sourceBindingID modeluuid.UUID) ([]coremodel.AllowlistSender, error)
	DeleteBySourceBindingAndSenderKey(ctx context.Context, sourceBindingID modeluuid.UUID, senderKey string) (bool, error)
}

type ThreadComponentMappingRepository interface {
	Save(ctx context.Context, mapping *coremodel.ThreadComponentMapping) error
	GetByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) (*coremodel.ThreadComponentMapping, error)
	FindByChatComponentAndThreadID(ctx context.Context, chatID modeluuid.UUID, componentID modeluuid.UUID, componentThreadID string) (*coremodel.ThreadComponentMapping, error)
	ListByChatID(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.ThreadComponentMapping, error)
	DeleteByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) error
	DeleteByComponentID(ctx context.Context, componentID modeluuid.UUID) (int64, error)
}

type ThreadComponentStateRepository interface {
	Save(ctx context.Context, state *coremodel.ThreadComponentState) error
	GetByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) (*coremodel.ThreadComponentState, error)
	DeleteByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) error
	DeleteByComponentID(ctx context.Context, componentID modeluuid.UUID) (int64, error)
}

type MessageRepository interface {
	Append(ctx context.Context, message *coremodel.ThreadMessage) error
	ListByThreadID(ctx context.Context, threadID modeluuid.UUID) ([]coremodel.ThreadMessage, error)
	ListByThreadIDPage(ctx context.Context, threadID modeluuid.UUID, afterMessageID modeluuid.UUID, limit int) ([]coremodel.ThreadMessage, string, error)
	CountByThreadIDSince(ctx context.Context, threadID modeluuid.UUID, since *time.Time) (int64, error)
	DeleteByThreadID(ctx context.Context, threadID modeluuid.UUID) (int64, error)
}

type ArtifactRepository interface {
	Append(ctx context.Context, artifact *coremodel.Artifact) error
	ListByMessageID(ctx context.Context, messageID modeluuid.UUID) ([]coremodel.Artifact, error)
	DeleteByThreadID(ctx context.Context, threadID modeluuid.UUID) (int64, error)
}

type TrustedControllerRepository interface {
	Save(ctx context.Context, controller *coremodel.TrustedController) error
	GetByFingerprint(ctx context.Context, fingerprint string) (*coremodel.TrustedController, error)
	List(ctx context.Context) ([]coremodel.TrustedController, error)
	RevokeByFingerprint(ctx context.Context, fingerprint string) (bool, error)
}

type ScheduledJobRepository interface {
	Save(ctx context.Context, job *coremodel.ScheduledJob) error
	List(ctx context.Context) ([]coremodel.ScheduledJob, error)
	ListDue(ctx context.Context, now time.Time) ([]coremodel.ScheduledJob, error)
	DeleteByName(ctx context.Context, name string) (bool, error)
}
