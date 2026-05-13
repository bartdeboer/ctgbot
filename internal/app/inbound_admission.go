package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/inbound"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

// AdmitInbound is the app-owned admission use case for inbound provider events.
// It answers: is this provider channel bound to a visible chat, and which
// source-bound filters should inspect the event before broker routes it?
func (s *service) AdmitInbound(ctx context.Context, event component.InboundEvent) (inbound.Admission, error) {
	externalChannelID := strings.TrimSpace(event.Payload.ProviderChannelID)
	if externalChannelID == "" {
		return inbound.Admission{}, fmt.Errorf("missing inbound provider channel id")
	}
	if s == nil || s.Storage == nil {
		return inbound.Admission{}, fmt.Errorf("missing inbound channel storage")
	}

	sourceBinding, err := s.Storage.ChatComponents().FindByComponentRoleAndExternalChannelID(
		ctx,
		event.ComponentID,
		coremodel.ChatComponentRoleSource,
		externalChannelID,
	)
	if err != nil {
		return inbound.Admission{}, err
	}
	if sourceBinding == nil {
		if err := s.RecordInboundDrop(ctx, event); err != nil {
			return inbound.Admission{}, err
		}
		return inbound.Admission{Rejected: inbound.Reject(event, nil, nil, inbound.RejectionDrop, "no-source-binding")}, nil
	}

	chat, err := s.Storage.Chats().GetByID(ctx, sourceBinding.ChatID)
	if err != nil {
		return inbound.Admission{}, err
	}
	if chat == nil {
		return inbound.Admission{Rejected: inbound.Reject(event, nil, sourceBinding, inbound.RejectionDrop, "missing-chat")}, nil
	}
	if !chat.Enabled {
		return inbound.Admission{Rejected: inbound.Reject(event, chat, sourceBinding, inbound.RejectionDrop, "chat-disabled")}, nil
	}
	hasRelay, err := s.hasRelayBinding(ctx, chat.ID)
	if err != nil {
		return inbound.Admission{}, err
	}
	if !hasRelay {
		return inbound.Admission{Rejected: inbound.Reject(event, chat, sourceBinding, inbound.RejectionDrop, "no-relay-binding")}, nil
	}

	channel := inbound.Channel{Chat: *chat, SourceBinding: *sourceBinding}
	filters, rejection, err := s.inboundFilters(ctx, channel, event)
	if err != nil || rejection != nil {
		return inbound.Admission{Channel: channel, Rejected: rejection}, err
	}
	return inbound.Admission{Channel: channel, Filters: filters}, nil
}

func (s *service) hasRelayBinding(ctx context.Context, chatID modeluuid.UUID) (bool, error) {
	if chatID.IsNull() {
		return false, nil
	}
	if s == nil || s.Storage == nil {
		return false, fmt.Errorf("missing inbound channel storage")
	}
	bindings, err := s.Storage.ChatComponents().ListEnabledByChatID(ctx, chatID)
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

func (s *service) inboundFilters(ctx context.Context, channel inbound.Channel, event component.InboundEvent) ([]inbound.Filterer, *inbound.Rejection, error) {
	if channel.SourceBinding.ID.IsNull() {
		return nil, nil, nil
	}
	if s == nil || s.Storage == nil {
		return nil, inbound.Reject(event, &channel.Chat, &channel.SourceBinding, inbound.RejectionQuarantine, "inbound-filter-config-error", "error=missing storage"), nil
	}
	bindings, err := s.Storage.InboundFilterBindings().ListEnabledBySourceBindingID(ctx, channel.SourceBinding.ID)
	if err != nil {
		return nil, nil, err
	}
	filters := make([]inbound.Filterer, 0, len(bindings))
	for _, binding := range bindings {
		filter, rejection, err := s.resolveInboundFilter(ctx, channel, event, binding)
		if err != nil || rejection != nil {
			return nil, rejection, err
		}
		filters = append(filters, filter)
	}
	return filters, nil, nil
}

func (s *service) resolveInboundFilter(ctx context.Context, channel inbound.Channel, event component.InboundEvent, binding coremodel.InboundFilterBinding) (inbound.Filterer, *inbound.Rejection, error) {
	loaded, err := s.ResolveComponent(ctx, binding.FilterComponentID)
	if err != nil {
		return nil, inbound.Reject(event, &channel.Chat, &channel.SourceBinding, inbound.RejectionQuarantine, "inbound-filter-config-error", "component="+binding.FilterComponentID.String(), "error="+err.Error()), nil
	}
	if loaded == nil {
		return nil, inbound.Reject(event, &channel.Chat, &channel.SourceBinding, inbound.RejectionQuarantine, "inbound-filter-config-error", "component="+binding.FilterComponentID.String(), "error=target component not found"), nil
	}
	filter, ok := loaded.Component.(inbound.Filterer)
	if !ok {
		return nil, inbound.Reject(event, &channel.Chat, &channel.SourceBinding, inbound.RejectionQuarantine, "inbound-filter-config-error", "component="+loaded.Registration.Ref(), "error=target does not implement inbound.Filterer"), nil
	}
	return filter, nil, nil
}

func (s *service) RecordInboundDrop(ctx context.Context, event component.InboundEvent) error {
	externalChannelID := strings.TrimSpace(event.Payload.ProviderChannelID)
	if externalChannelID == "" {
		return fmt.Errorf("missing inbound provider channel id")
	}
	if s == nil || s.Storage == nil {
		return fmt.Errorf("missing inbound drop storage")
	}
	actor := event.Payload.ResolvedActor()
	now := time.Now()

	drop, err := s.Storage.InboundDrops().GetByComponentAndExternalChannelID(ctx, event.ComponentID, externalChannelID)
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
	drop.LastTextPreview = inboundDropPreview(event.Payload.Text.Text)
	drop.MessageCount++
	if drop.FirstSeenAt.IsZero() {
		drop.FirstSeenAt = now
	}
	drop.LastSeenAt = now
	return s.Storage.InboundDrops().Save(ctx, drop)
}

func inboundDropPreview(text string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	const max = 160
	if len([]rune(text)) <= max {
		return text
	}
	runes := []rune(text)
	return string(runes[:max]) + "…"
}
