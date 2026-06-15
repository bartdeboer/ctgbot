package broker

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/component"
	componentbroker "github.com/bartdeboer/ctgbot/internal/component/broker"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	"github.com/bartdeboer/ctgbot/internal/inbound"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

// App is the broker's application boundary.
//
// Keep this as a composition of small ports. The broker coordinates inbound
// events and turns; the app side owns persistence, policy, and component
// loading. Splitting the boundary by capability makes that dependency honest
// without changing the concrete app service implementation.
type App interface {
	InboundAdmitter
	ChatThreadRegistry
	MessageLog
	ChatComponentRegistry
	InboundSourceRegistry
	CommandSurfaceRegistry
	RelayResolver
	ComponentThreadRegistry
	DroppedEventStore
	ComponentResolver
	WorkspacePolicy
}

// InboundAdmitter decides whether an inbound provider event may enter a chat.
type InboundAdmitter interface {
	AdmitInbound(ctx context.Context, event component.InboundEvent) (inbound.Admission, error)
}

// ChatThreadRegistry resolves chat/thread identity and creates provider-backed
// threads when a source binding first appears.
type ChatThreadRegistry interface {
	Chat(ctx context.Context, chatID modeluuid.UUID) (*coremodel.Chat, error)
	Thread(ctx context.Context, threadID modeluuid.UUID) (*coremodel.Thread, error)
	EnsureThread(ctx context.Context, binding coremodel.ChatComponent, componentThreadID string) (*coremodel.Thread, error)
}

// MessageLog owns durable conversation messages and iteration for memory/search.
type MessageLog interface {
	ThreadMessages(ctx context.Context, threadID modeluuid.UUID) ([]coremodel.ThreadMessage, error)
	ForEachMessage(ctx context.Context, scope component.MessageScope, visit component.MessageVisitor) error
	StoreInboundMessage(ctx context.Context, inbound component.ResolvedInbound) (*coremodel.ThreadMessage, error)
	StoreOutboundMessage(ctx context.Context, message *coremodel.ThreadMessage, attachments []message.Media) error
}

// ChatComponentRegistry lists components enabled for a specific chat runtime.
type ChatComponentRegistry interface {
	EnabledChatComponents(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.ChatComponent, error)
}

// InboundSourceRegistry lists source components that should be polled globally.
type InboundSourceRegistry interface {
	EnabledInboundSources(ctx context.Context) ([]component.InboundSource, error)
}

// CommandSurfaceRegistry exposes the command authority surface for a chat.
type CommandSurfaceRegistry interface {
	CommandSurfaces(ctx context.Context, chat coremodel.Chat, inbound component.ResolvedInboundQueuer, actions componentbroker.Actions) ([]component.CommandSurface, error)
}

// RelayResolver resolves provider-specific outbound targets for a thread.
type RelayResolver interface {
	RelayTarget(ctx context.Context, threadID modeluuid.UUID, binding coremodel.ChatComponent) (*message.ChatTarget, bool, error)
}

// ComponentThreadRegistry owns provider thread ids for each ctgbot thread/component pair.
type ComponentThreadRegistry interface {
	ComponentThreadID(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) (string, bool, error)
	BindComponentThreadID(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID, componentThreadID string) error
}

// DroppedEventStore owns rejected inbound events and their replay lifecycle.
type DroppedEventStore interface {
	DropEvent(ctx context.Context, rejection *inbound.Rejection) (*coremodel.DroppedEvent, error)
	DropNoticeID(ctx context.Context, drop *coremodel.DroppedEvent) string
	ResolveDroppedEventID(ctx context.Context, ref string) (modeluuid.UUID, error)
	DroppedEvent(ctx context.Context, id modeluuid.UUID) (*coremodel.DroppedEvent, error)
	ListDroppedEvents(ctx context.Context, limit int) ([]coremodel.DroppedEvent, error)
	SaveDroppedEvent(ctx context.Context, drop *coremodel.DroppedEvent) error
}

// ComponentResolver loads registered component implementations by id.
type ComponentResolver interface {
	ResolveComponent(ctx context.Context, componentID modeluuid.UUID) (*component.Loaded, error)
}

// WorkspacePolicy resolves the filesystem and hostbridge trust boundary for a chat.
type WorkspacePolicy interface {
	ResolveChatWorkspace(ctx context.Context, chat coremodel.Chat) (string, error)
	ResolveChatHostbridgeAliases(ctx context.Context, chat coremodel.Chat) (map[string]hostbridgeserver.Alias, error)
}

// ThreadInstructionProvider optionally supplies operator-authored prompt text
// for a specific thread. These instructions are presentation only; they do not
// grant command authority.
type ThreadInstructionProvider interface {
	ThreadExtraInstructions(ctx context.Context, threadID modeluuid.UUID) (string, error)
}
