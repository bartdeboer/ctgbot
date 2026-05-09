package messaging

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

// Service is the core thread-oriented messaging contract.
//
// Different adapters should call this same interface:
//
// - hostbridge commands
// - remote HTTP clients
// - future web clients
// - possible agent-facing command surfaces
type Service interface {
	ListThreads(ctx context.Context, actor coremodel.Actor, req ListThreadsRequest) ([]ThreadSummary, error)
	ListMessages(ctx context.Context, actor coremodel.Actor, threadID modeluuid.UUID, req ListMessagesRequest) (MessagePage, error)
	SendMessage(ctx context.Context, actor coremodel.Actor, threadID modeluuid.UUID, req SendMessageRequest) (*SendMessageResult, error)
}

// LocalActions extends the core service with the extra conveniences needed by
// the built-in local thread command surface.
type LocalActions interface {
	Service
	ResolveThreadRef(ctx context.Context, ref string) (modeluuid.UUID, error)
	ActorForThread(ctx context.Context, threadID modeluuid.UUID) (coremodel.Actor, error)
}

// ResolvedInbound is the generic broker-facing inbound shape once chat/thread
// routing is already known.
type ResolvedInbound struct {
	Chat        coremodel.Chat
	Thread      coremodel.Thread
	ComponentID modeluuid.UUID
	ExternalID  string
	Payload     message.InboundPayload
	Metadata    []string
}

// DeliveryResult is the broker delivery outcome for a resolved inbound
// message.
type DeliveryResult struct {
	Inbound  *coremodel.ThreadMessage
	Outbound []coremodel.ThreadMessage
}

// Deliverer accepts a resolved inbound message and runs the normal thread turn
// path for it.
type Deliverer interface {
	HandleResolvedInbound(ctx context.Context, inbound ResolvedInbound) (DeliveryResult, error)
}
