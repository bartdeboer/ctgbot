package broker

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	component "github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/inbound"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type InboundRejectionAction string

const (
	InboundRejectionDrop InboundRejectionAction = "drop"
	// InboundRejectionNotifyOnly is reserved for a future admission stage that
	// notifies an operator without routing the inbound message. No current stage
	// produces it.
	InboundRejectionNotifyOnly InboundRejectionAction = "notify_only"
	InboundRejectionQuarantine InboundRejectionAction = "quarantine"
)

type InboundRejection struct {
	Action        InboundRejectionAction
	Event         component.InboundEvent
	Chat          *coremodel.Chat
	SourceBinding *coremodel.ChatComponent
	Reason        string
	NoticeText    string
	Details       []string
}

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

	return inbound.NewFilterChain(ctx, filters)
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

func (b *Broker) reject(event component.InboundEvent, chat *coremodel.Chat, sourceBinding *coremodel.ChatComponent, action InboundRejectionAction, reason string, details ...string) *InboundRejection {
	return &InboundRejection{
		Action:        action,
		Event:         event,
		Chat:          chat,
		SourceBinding: sourceBinding,
		Reason:        strings.TrimSpace(reason),
		Details:       append([]string(nil), details...),
	}
}
func (b *Broker) handleInboundRejection(ctx context.Context, rejection *InboundRejection) {
	if rejection == nil {
		return
	}
	dropped, err := b.DropEvent(ctx, rejection)
	if err != nil {
		b.logf("dropped event persistence failed component=%s reason=%s err=%v", rejection.Event.ComponentID, rejection.Reason, err)
	}
	dropEvent := rejection.Event
	actor := dropEvent.Payload.ResolvedActor()
	details := strings.Join(rejection.Details, " ")
	b.logf(
		"inbound dropped component=%s external_channel=%q external_thread=%q reason=%s actor_id=%q actor_label=%q chat_label=%q preview=%q details=%q",
		dropEvent.ComponentID,
		strings.TrimSpace(dropEvent.Payload.ProviderChannelID),
		strings.TrimSpace(dropEvent.Payload.ProviderThreadID),
		rejection.Reason,
		strings.TrimSpace(actor.ID),
		strings.TrimSpace(actor.Label),
		strings.TrimSpace(dropEvent.Payload.ChatLabel),
		inboundPreview(dropEvent.Payload.Text.Text),
		details,
	)
	if err := b.sendInboundRejectionNotice(ctx, rejection, dropped); err != nil {
		b.logf("inbound rejection notice failed component=%s external_channel=%q reason=%s err=%v", dropEvent.ComponentID, strings.TrimSpace(dropEvent.Payload.ProviderChannelID), rejection.Reason, err)
	}
	b.maybeHandleInboundInitReply(ctx, rejection)
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

func (b *Broker) maybeHandleInboundInitReply(ctx context.Context, rejection *InboundRejection) {
	if rejection == nil {
		return
	}
	switch rejection.Reason {
	case "no-source-binding", "missing-chat", "chat-disabled":
	default:
		return
	}
	event := rejection.Event
	if !isInitCommand(event.Payload.Text.Text) {
		return
	}
	if err := b.sendInboundInitReply(ctx, rejection); err != nil {
		b.logf("inbound init reply failed component=%s external_channel=%q reason=%s err=%v", event.ComponentID, strings.TrimSpace(event.Payload.ProviderChannelID), rejection.Reason, err)
	}
}

func (b *Broker) sendInboundInitReply(ctx context.Context, rejection *InboundRejection) error {
	resolver := b.resolver()
	if resolver == nil || rejection == nil {
		return nil
	}
	event := rejection.Event
	loaded, err := resolver.ResolveComponent(ctx, event.ComponentID)
	if err != nil {
		return err
	}
	relay, ok := loaded.Component.(component.OutboundRelay)
	if !ok {
		return nil
	}

	componentRef := loaded.Registration.Ref()
	if componentRef == "" {
		componentRef = event.ComponentID.String()
	}
	externalChannelID := strings.TrimSpace(event.Payload.ProviderChannelID)
	externalThreadID := strings.TrimSpace(event.Payload.ProviderThreadID)
	label := strings.TrimSpace(event.Payload.ChatLabel)

	status := "chat is not enabled"
	if rejection.Chat == nil {
		status = "chat is not bound"
	}

	lines := []string{
		status,
		"component: " + componentRef,
		"external_channel_id: " + externalChannelID,
		"external_thread_id: " + externalThreadID,
		"",
		"Console:",
	}
	switch {
	case rejection.Chat != nil:
		lines = append(lines,
			"ctgbot config chat "+rejection.Chat.ID.String()+" set chat.enabled true",
		)
	default:
		bind := "ctgbot chat bind " + componentRef + " " + externalChannelID
		if label != "" {
			bind += " " + strconv.Quote(label)
		}
		lines = append(lines, bind)
	}

	return relay.Send(ctx, message.OutboundPayload{
		ProviderChannelID: externalChannelID,
		ProviderThreadID:  externalThreadID,
		Text:              message.TextMessage{Text: strings.Join(lines, "\n")},
	})
}

func isInitCommand(text string) bool {
	argv, ok := commandArgv(text)
	return ok && len(argv) == 1 && strings.EqualFold(argv[0], "init")
}

func inboundPreview(text string) string {
	text = strings.TrimSpace(text)
	const maxPreview = 240
	if len(text) <= maxPreview {
		return text
	}
	return text[:maxPreview]
}
