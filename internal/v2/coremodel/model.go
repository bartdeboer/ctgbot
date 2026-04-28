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
	ID    modeluuid.UUID `gorm:"primaryKey"`
	Label string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Thread is ctgbot's canonical conversation thread.
type Thread struct {
	ID     modeluuid.UUID `gorm:"primaryKey"`
	ChatID modeluuid.UUID `gorm:"index"`
	Label  string

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
