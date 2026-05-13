package broker

import (
	"context"
	"fmt"
	"strings"
	"time"

	component "github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/inbound"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func (b *Broker) AllowedChannel(ctx context.Context, event component.InboundEvent) (inbound.Channel, *InboundRejection, error) {
	externalChannelID := strings.TrimSpace(event.Payload.ProviderChannelID)
	if externalChannelID == "" {
		return inbound.Channel{}, nil, fmt.Errorf("missing inbound provider channel id")
	}
	storage := b.repository()
	if storage == nil {
		return inbound.Channel{}, nil, fmt.Errorf("missing inbound channel storage")
	}

	sourceBinding, err := storage.ChatComponents().FindByComponentRoleAndExternalChannelID(
		ctx,
		event.ComponentID,
		coremodel.ChatComponentRoleSource,
		externalChannelID,
	)
	if err != nil {
		return inbound.Channel{}, nil, err
	}
	if sourceBinding == nil {
		if err := b.recordInboundDrop(ctx, event); err != nil {
			return inbound.Channel{}, nil, err
		}
		return inbound.Channel{}, b.reject(event, nil, nil, InboundRejectionDrop, "no-source-binding"), nil
	}

	chat, err := storage.Chats().GetByID(ctx, sourceBinding.ChatID)
	if err != nil {
		return inbound.Channel{}, nil, err
	}
	if chat == nil {
		return inbound.Channel{}, b.reject(event, nil, sourceBinding, InboundRejectionDrop, "missing-chat"), nil
	}
	if !chat.Enabled {
		return inbound.Channel{}, b.reject(event, chat, sourceBinding, InboundRejectionDrop, "chat-disabled"), nil
	}
	hasRelay, err := b.hasRelayBinding(ctx, chat.ID)
	if err != nil {
		return inbound.Channel{}, nil, err
	}
	if !hasRelay {
		return inbound.Channel{}, b.reject(event, chat, sourceBinding, InboundRejectionDrop, "no-relay-binding"), nil
	}
	return inbound.Channel{Chat: *chat, SourceBinding: *sourceBinding}, nil, nil
}

func (b *Broker) hasRelayBinding(ctx context.Context, chatID modeluuid.UUID) (bool, error) {
	if chatID.IsNull() {
		return false, nil
	}
	storage := b.repository()
	if storage == nil {
		return false, fmt.Errorf("missing inbound channel storage")
	}
	bindings, err := storage.ChatComponents().ListEnabledByChatID(ctx, chatID)
	if err != nil {
		return false, err
	}
	for _, binding := range bindings {
		if binding.Role == coremodel.ChatComponentRoleRelay {
			return true, nil
		}
	}
	return false, nil
}

func (b *Broker) recordInboundDrop(ctx context.Context, event component.InboundEvent) error {
	externalChannelID := strings.TrimSpace(event.Payload.ProviderChannelID)
	if externalChannelID == "" {
		return fmt.Errorf("missing inbound provider channel id")
	}
	storage := b.repository()
	if storage == nil {
		return fmt.Errorf("missing inbound drop storage")
	}
	actor := event.Payload.ResolvedActor()
	now := time.Now()

	drop, err := storage.InboundDrops().GetByComponentAndExternalChannelID(ctx, event.ComponentID, externalChannelID)
	if err != nil {
		return err
	}
	if drop == nil {
		drop = &coremodel.InboundDrop{
			ComponentID:       event.ComponentID,
			ExternalChannelID: externalChannelID,
			FirstSeenAt:       now,
		}
	}
	drop.ExternalThreadID = strings.TrimSpace(event.Payload.ProviderThreadID)
	drop.ChatLabel = strings.TrimSpace(event.Payload.ChatLabel)
	drop.ActorID = strings.TrimSpace(actor.ID)
	drop.ActorLabel = strings.TrimSpace(actor.Label)
	drop.LastTextPreview = inboundPreview(event.Payload.Text.Text)
	drop.MessageCount++
	if drop.FirstSeenAt.IsZero() {
		drop.FirstSeenAt = now
	}
	drop.LastSeenAt = now
	return storage.InboundDrops().Save(ctx, drop)
}
