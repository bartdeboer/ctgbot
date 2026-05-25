package coremodel

import (
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
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
	MessageKindMessage    MessageKind = "message"
	MessageKindEvent      MessageKind = "event"
	MessageKindReasoning  MessageKind = "reasoning"
	MessageKindProgress   MessageKind = "progress"
	MessageKindToolCall   MessageKind = "tool_call"
	MessageKindToolResult MessageKind = "tool_result"
)

type MessageRole string

const (
	MessageRoleUser   MessageRole = "user"
	MessageRoleAgent  MessageRole = "agent"
	MessageRoleSystem MessageRole = "system"
)

type Actor struct {
	ID    string
	Label string
	Roles []simplerbac.Role
}

func (a Actor) Resolved() Actor {
	if strings.TrimSpace(a.ID) == "" {
		a.ID = strings.TrimSpace(a.Label)
	}
	if strings.TrimSpace(a.Label) == "" {
		a.Label = strings.TrimSpace(a.ID)
	}
	return a
}

func (a Actor) HasRole(role simplerbac.Role) bool {
	if role == "" {
		return false
	}
	for _, candidate := range a.Roles {
		if candidate == role {
			return true
		}
	}
	return false
}

type Chat struct {
	ID        modeluuid.UUID `gorm:"primaryKey"`
	Label     string
	Workspace string
	Enabled   bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

type Thread struct {
	ID     modeluuid.UUID `gorm:"primaryKey"`
	ChatID modeluuid.UUID `gorm:"index"`
	Label  string

	VoiceReplyToVoiceInput bool
	VoiceOutput            bool
	VoiceLanguage          string
	VoiceName              string
	VoiceModel             string
	VoiceDeviceTarget      string

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
	ID                modeluuid.UUID    `gorm:"primaryKey"`
	ChatID            modeluuid.UUID    `gorm:"index;uniqueIndex:idx_chat_component_role"`
	ComponentID       modeluuid.UUID    `gorm:"uniqueIndex:idx_chat_component_role"`
	Role              ChatComponentRole `gorm:"uniqueIndex:idx_chat_component_role"`
	ExternalChannelID string
	Enabled           bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

type InboundFilterBinding struct {
	ID                modeluuid.UUID `gorm:"primaryKey"`
	SourceBindingID   modeluuid.UUID `gorm:"index;uniqueIndex:idx_inbound_filter_binding"`
	FilterComponentID modeluuid.UUID `gorm:"uniqueIndex:idx_inbound_filter_binding"`
	Enabled           bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

type InboundDrop struct {
	ID                modeluuid.UUID `gorm:"primaryKey"`
	ComponentID       modeluuid.UUID `gorm:"index;uniqueIndex:idx_inbound_drop_external_channel"`
	ExternalChannelID string         `gorm:"uniqueIndex:idx_inbound_drop_external_channel"`
	ExternalThreadID  string
	ChatLabel         string
	ActorID           string
	ActorLabel        string
	LastTextPreview   string
	MessageCount      int64
	FirstSeenAt       time.Time
	LastSeenAt        time.Time
}

type DroppedEvent struct {
	ID                modeluuid.UUID `gorm:"primaryKey"`
	CreatedAt         time.Time
	ExpiresAt         time.Time `gorm:"index"`
	Status            string    `gorm:"index"`
	Action            string
	Reason            string
	ChatID            modeluuid.UUID `gorm:"index"`
	SourceBindingID   modeluuid.UUID `gorm:"index"`
	ComponentID       modeluuid.UUID `gorm:"index"`
	ProviderChannelID string
	ProviderThreadID  string
	ProviderMessageID string
	SenderKey         string `gorm:"index"`
	SenderLabel       string
	Subject           string
	Preview           string
	EventJSON         string
}

type AllowlistSender struct {
	ID              modeluuid.UUID `gorm:"primaryKey"`
	SourceBindingID modeluuid.UUID `gorm:"index;uniqueIndex:idx_allowlist_sender_scope"`
	SenderKey       string         `gorm:"uniqueIndex:idx_allowlist_sender_scope"`
	SenderLabel     string
	CreatedAt       time.Time
	UpdatedAt       time.Time
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

type ThreadComponentState struct {
	ID          modeluuid.UUID `gorm:"primaryKey"`
	ThreadID    modeluuid.UUID `gorm:"index;uniqueIndex:idx_thread_component_state"`
	ComponentID modeluuid.UUID `gorm:"uniqueIndex:idx_thread_component_state"`
	StateJSON   string

	CreatedAt time.Time
	UpdatedAt time.Time
}

type ThreadMessage struct {
	ID           modeluuid.UUID `gorm:"primaryKey"`
	ChatID       modeluuid.UUID `gorm:"index"`
	ThreadID     modeluuid.UUID `gorm:"index"`
	Direction    MessageDirection
	Role         MessageRole
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

func (m ThreadMessage) ResolvedRole() MessageRole {
	return m.Role
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
	Content      []byte `gorm:"-"`
	StorageKind  string
	StoragePath  string
	Size         int64
	SHA256       string
	MetadataJSON string

	CreatedAt time.Time
	UpdatedAt time.Time
}
