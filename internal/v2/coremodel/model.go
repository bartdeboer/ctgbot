// Package coremodel defines the provider-neutral conversation model for the
// next ctgbot broker.
package coremodel

import (
	"time"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type MessageDirection string

const (
	DirectionInbound  MessageDirection = "inbound"
	DirectionOutbound MessageDirection = "outbound"
)

type MessageKind string

const (
	MessageKindUser   MessageKind = "user"
	MessageKindAgent  MessageKind = "agent"
	MessageKindSystem MessageKind = "system"
	MessageKindEvent  MessageKind = "event"
	MessageKindFinal  MessageKind = "final"
)

// Chat is ctgbot's canonical chat record. Provider chats can project into this
// record, but the conversation stream belongs to ctgbot.
type Chat struct {
	ID modeluuid.UUID `gorm:"primaryKey"`

	ProviderType   string `gorm:"uniqueIndex:idx_v2_chat_provider"`
	ProviderChatID string `gorm:"uniqueIndex:idx_v2_chat_provider"`

	Label   string
	Enabled bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Thread is ctgbot's canonical conversation thread.
type Thread struct {
	ID modeluuid.UUID `gorm:"primaryKey"`

	ChatID           modeluuid.UUID `gorm:"index;uniqueIndex:idx_v2_thread_provider"`
	ProviderThreadID string         `gorm:"uniqueIndex:idx_v2_thread_provider"`

	Label string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Component is a registered component type known to ctgbot.
type Component struct {
	ID modeluuid.UUID `gorm:"primaryKey"`

	Type    string `gorm:"uniqueIndex"`
	Label   string
	Enabled bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// ComponentProfile is a reusable profile for one component type.
type ComponentProfile struct {
	ID modeluuid.UUID `gorm:"primaryKey"`

	ComponentType string `gorm:"uniqueIndex:idx_v2_component_profile"`
	ProfileName   string `gorm:"uniqueIndex:idx_v2_component_profile"`

	Label   string
	Enabled bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// ChatComponent binds a component profile to a chat.
type ChatComponent struct {
	ID modeluuid.UUID `gorm:"primaryKey"`

	ChatID        modeluuid.UUID `gorm:"index;uniqueIndex:idx_v2_chat_component"`
	ComponentType string         `gorm:"uniqueIndex:idx_v2_chat_component"`
	ProfileName   string         `gorm:"uniqueIndex:idx_v2_chat_component"`

	Enabled bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// ThreadMessage is the canonical stream item routed into or produced by a
// thread. Provider-specific identifiers live at the edge as source metadata.
type ThreadMessage struct {
	ID       modeluuid.UUID `gorm:"primaryKey"`
	ChatID   modeluuid.UUID `gorm:"index"`
	ThreadID modeluuid.UUID `gorm:"index"`

	Direction MessageDirection
	Kind      MessageKind

	SourceType string
	ExternalID string `gorm:"index"`

	ActorID    string
	ActorLabel string
	Text       string

	MetadataJSON string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Artifact is a secondary payload owned by a thread message.
type Artifact struct {
	ID        modeluuid.UUID `gorm:"primaryKey"`
	ChatID    modeluuid.UUID `gorm:"index"`
	ThreadID  modeluuid.UUID `gorm:"index"`
	MessageID modeluuid.UUID `gorm:"index"`

	Filename     string
	ContentType  string
	Syntax       string
	Path         string
	MetadataJSON string

	CreatedAt time.Time
	UpdatedAt time.Time
}
