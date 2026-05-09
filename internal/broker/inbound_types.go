package broker

import (
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

// ResolvedInbound is the generic inbound shape once chat/thread routing is
// already known.
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
