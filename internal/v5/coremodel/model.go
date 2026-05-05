package coremodel

import (
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type ChatComponentRole string

const (
	ChatComponentRoleSource  ChatComponentRole = "source"
	ChatComponentRoleRelay   ChatComponentRole = "relay"
	ChatComponentRoleAgent   ChatComponentRole = "agent"
	ChatComponentRoleCommand ChatComponentRole = "command"
)

type MessageDirection string

const (
	MessageDirectionInbound  MessageDirection = "inbound"
	MessageDirectionOutbound MessageDirection = "outbound"
)

type MessageKind string

const (
	MessageKindUser   MessageKind = "user"
	MessageKindAgent  MessageKind = "agent"
	MessageKindSystem MessageKind = "system"
	MessageKindEvent  MessageKind = "event"
)

type Chat struct {
	ID        modeluuid.UUID `gorm:"primaryKey"`
	Label     string
	Workspace string
	Enabled   bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

type Thread struct {
	ID                   modeluuid.UUID `gorm:"primaryKey"`
	ChatID               modeluuid.UUID `gorm:"index"`
	Label                string
	CodexModel           string
	CodexReasoningEffort string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Component is a registered system component instance.
//
// Type identifies the code plugin, while Name identifies the configured
// registration of that plugin in the current ctgbot runtime.
type Component struct {
	ID        modeluuid.UUID `gorm:"primaryKey"`
	Type      string         `gorm:"index;uniqueIndex:idx_component_type_name"`
	Name      string         `gorm:"uniqueIndex:idx_component_type_name"`
	Runtime   string
	HomePath  string
	Label     string
	Enabled   bool
	IsDefault bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

func DefaultComponentName(componentType string) string {
	return strings.TrimSpace(componentType)
}

func ComponentRef(componentType string, name string) string {
	componentType = strings.TrimSpace(componentType)
	name = strings.TrimSpace(name)
	if componentType == "" {
		return ""
	}
	if name == "" || name == DefaultComponentName(componentType) {
		return componentType
	}
	return componentType + "/" + name
}

func (c Component) Ref() string {
	return ComponentRef(c.Type, c.Name)
}

type ChatComponent struct {
	ID             modeluuid.UUID    `gorm:"primaryKey"`
	ChatID         modeluuid.UUID    `gorm:"index;uniqueIndex:idx_chat_component_role"`
	ComponentID    modeluuid.UUID    `gorm:"uniqueIndex:idx_chat_component_role"`
	Role           ChatComponentRole `gorm:"uniqueIndex:idx_chat_component_role"`
	ExternalChatID string
	Enabled        bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

type ThreadComponentMapping struct {
	ID                modeluuid.UUID `gorm:"primaryKey"`
	ThreadID          modeluuid.UUID `gorm:"index;uniqueIndex:idx_thread_component_binding"`
	ChatID            modeluuid.UUID `gorm:"index;uniqueIndex:idx_component_thread_mapping"`
	ComponentID       modeluuid.UUID `gorm:"uniqueIndex:idx_thread_component_binding;uniqueIndex:idx_component_thread_mapping"`
	ComponentThreadID string         `gorm:"index;uniqueIndex:idx_component_thread_mapping"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

type ThreadMessage struct {
	ID           modeluuid.UUID `gorm:"primaryKey"`
	ChatID       modeluuid.UUID `gorm:"index"`
	ThreadID     modeluuid.UUID `gorm:"index"`
	Direction    MessageDirection
	Kind         MessageKind
	ComponentID  modeluuid.UUID `gorm:"index"`
	ExternalID   string         `gorm:"index"`
	ActorID      string
	ActorLabel   string
	Text         string
	MetadataJSON string

	CreatedAt time.Time
	UpdatedAt time.Time
}

type Artifact struct {
	ID           modeluuid.UUID `gorm:"primaryKey"`
	ChatID       modeluuid.UUID `gorm:"index"`
	ThreadID     modeluuid.UUID `gorm:"index"`
	MessageID    modeluuid.UUID `gorm:"index"`
	ComponentID  modeluuid.UUID `gorm:"index"`
	Filename     string
	ContentType  string
	Syntax       string
	Content      []byte
	MetadataJSON string

	CreatedAt time.Time
	UpdatedAt time.Time
}
