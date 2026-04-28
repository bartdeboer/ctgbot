// Package broker sketches the v2 routing layer.
package broker

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v2/component"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v2/repository"
)

type Broker struct {
	storage    repository.Storage
	components *component.Registry
}

func New(storage repository.Storage, components *component.Registry) *Broker {
	return &Broker{storage: storage, components: components}
}

func (b *Broker) Components() *component.Registry {
	if b == nil {
		return nil
	}
	return b.components
}

// RouteInboundEvent records an external component event as a canonical inbound
// thread message. Agent execution and outbound delivery are deliberately left
// for later passes.
func (b *Broker) RouteInboundEvent(ctx context.Context, event component.InboundEvent) (*coremodel.ThreadMessage, error) {
	if b == nil || b.storage == nil {
		return nil, fmt.Errorf("missing broker storage")
	}
	if event.ChatID.IsNull() {
		return nil, fmt.Errorf("missing chat id")
	}
	if event.ThreadID.IsNull() {
		return nil, fmt.Errorf("missing thread id")
	}

	message := &coremodel.ThreadMessage{
		ChatID:     event.ChatID,
		ThreadID:   event.ThreadID,
		Direction:  coremodel.DirectionInbound,
		Kind:       coremodel.MessageKindUser,
		SourceType: event.SourceType,
		ExternalID: event.ExternalID,
		ActorID:    event.Actor.ID,
		ActorLabel: event.Actor.Label,
		Text:       event.Text,
	}
	if event.EventType != "" && event.EventType != "message.received" {
		message.Kind = coremodel.MessageKindEvent
	}
	if err := b.storage.Messages().Append(ctx, message); err != nil {
		return nil, err
	}
	return message, nil
}

func requireThreadID(threadID modeluuid.UUID) error {
	if threadID.IsNull() {
		return fmt.Errorf("missing thread id")
	}
	return nil
}
