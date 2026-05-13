package broker

import (
	"context"
	"strings"

	component "github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/inbound"
)

func (b *Broker) FilterChainForChannel(ctx context.Context, channel inbound.Channel) (inbound.FilterChain, *inbound.FilterResult, error) {
	var filters []inbound.Filterer

	if !channel.SourceBinding.ID.IsNull() {
		storage := b.repository()
		if storage == nil {
			failure := inbound.Quarantine(inbound.ChannelEvent{Channel: channel}, "inbound-filter-config-error", "error=missing storage")
			return inbound.FilterChain{}, &failure, nil
		}
		bindings, err := storage.InboundFilterBindings().ListEnabledBySourceBindingID(ctx, channel.SourceBinding.ID)
		if err != nil {
			return inbound.FilterChain{}, nil, err
		}
		for _, binding := range bindings {
			filter, failure, err := b.resolveInboundFilter(ctx, channel, binding)
			if err != nil || failure != nil {
				return inbound.FilterChain{}, failure, err
			}
			filters = append(filters, filter)
		}
	}

	return inbound.NewFilterChain(filters), nil, nil
}

func (b *Broker) resolveInboundFilter(ctx context.Context, channel inbound.Channel, binding coremodel.InboundFilterBinding) (inbound.Filterer, *inbound.FilterResult, error) {
	resolver := b.resolver()
	if resolver == nil {
		failure := inbound.Quarantine(inbound.ChannelEvent{Channel: channel}, "inbound-filter-config-error", "component="+binding.FilterComponentID.String(), "error=missing component resolver")
		return nil, &failure, nil
	}
	loaded, err := resolver.ResolveComponent(ctx, binding.FilterComponentID)
	if err != nil {
		failure := inbound.Quarantine(inbound.ChannelEvent{Channel: channel}, "inbound-filter-config-error", "component="+binding.FilterComponentID.String(), "error="+err.Error())
		return nil, &failure, nil
	}
	if loaded == nil {
		failure := inbound.Quarantine(inbound.ChannelEvent{Channel: channel}, "inbound-filter-config-error", "component="+binding.FilterComponentID.String(), "error=target component not found")
		return nil, &failure, nil
	}
	filter, ok := loaded.Component.(inbound.Filterer)
	if !ok {
		failure := inbound.Quarantine(inbound.ChannelEvent{Channel: channel}, "inbound-filter-config-error", "component="+loaded.Registration.Ref(), "error=target does not implement inbound.Filterer")
		return nil, &failure, nil
	}
	return filter, nil, nil
}

func (b *Broker) inboundFilterRejection(channel inbound.Channel, current component.InboundEvent, result inbound.FilterResult) *InboundRejection {
	rejectedEvent := result.Event
	if rejectedEvent.ComponentID.IsNull() {
		rejectedEvent = current
	}
	action := InboundRejectionDrop
	if result.Action == inbound.FilterActionQuarantine {
		action = InboundRejectionQuarantine
	}
	rejection := b.reject(rejectedEvent, &channel.Chat, &channel.SourceBinding, action, result.Reason, result.Details...)
	rejection.NoticeText = strings.TrimSpace(result.NoticeText)
	return rejection
}
