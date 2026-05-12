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

type AllowedChannel struct {
	Event         component.InboundEvent
	Chat          coremodel.Chat
	SourceBinding coremodel.ChatComponent
}

type InboundRejection struct {
	Action        InboundRejectionAction
	Event         component.InboundEvent
	Chat          *coremodel.Chat
	SourceBinding *coremodel.ChatComponent
	Reason        string
	NoticeText    string
	Details       []string
}

func contentFilters(filters ...inbound.Filter) []inbound.Filter {
	out := make([]inbound.Filter, 0, len(filters))
	for _, filter := range filters {
		if filter != nil {
			out = append(out, filter)
		}
	}
	return out
}

func (b *Broker) AllowedChannel(ctx context.Context, event component.InboundEvent) (AllowedChannel, *InboundRejection, error) {
	externalChatID := strings.TrimSpace(event.Payload.ProviderChatID)
	if externalChatID == "" {
		return AllowedChannel{}, nil, fmt.Errorf("missing inbound provider chat id")
	}
	storage := b.repository()
	if storage == nil {
		return AllowedChannel{}, nil, fmt.Errorf("missing inbound channel storage")
	}

	sourceBinding, err := storage.ChatComponents().FindByComponentRoleAndExternalChatID(
		ctx,
		event.ComponentID,
		coremodel.ChatComponentRoleSource,
		externalChatID,
	)
	if err != nil {
		return AllowedChannel{}, nil, err
	}
	if sourceBinding == nil {
		if err := b.recordInboundDrop(ctx, event); err != nil {
			return AllowedChannel{}, nil, err
		}
		return AllowedChannel{}, b.reject(event, nil, nil, InboundRejectionDrop, "no-source-binding"), nil
	}

	chat, err := storage.Chats().GetByID(ctx, sourceBinding.ChatID)
	if err != nil {
		return AllowedChannel{}, nil, err
	}
	if chat == nil {
		return AllowedChannel{}, b.reject(event, nil, sourceBinding, InboundRejectionDrop, "missing-chat"), nil
	}
	if !chat.Enabled {
		return AllowedChannel{}, b.reject(event, chat, sourceBinding, InboundRejectionDrop, "chat-disabled"), nil
	}
	hasRelay, err := b.hasRelayBinding(ctx, chat.ID)
	if err != nil {
		return AllowedChannel{}, nil, err
	}
	if !hasRelay {
		return AllowedChannel{}, b.reject(event, chat, sourceBinding, InboundRejectionDrop, "no-relay-binding"), nil
	}
	return AllowedChannel{Event: event, Chat: *chat, SourceBinding: *sourceBinding}, nil, nil
}

func (b *Broker) AllowedSender(ctx context.Context, channel AllowedChannel) (*InboundRejection, error) {
	_, _ = ctx, channel
	return nil, nil
}

func (b *Broker) FilteredMessage(ctx context.Context, channel AllowedChannel) (component.InboundEvent, *InboundRejection, error) {
	current := channel.Event
	for _, filter := range b.InboundFilters {
		if filter == nil {
			continue
		}
		input := inbound.FilterInput{
			Event:         current,
			Chat:          channel.Chat,
			SourceBinding: channel.SourceBinding,
		}
		result, err := filter.FilterInbound(ctx, input)
		if err != nil {
			return component.InboundEvent{}, nil, err
		}
		switch result.Action {
		case "", inbound.FilterActionPass:
			current = result.Event
			if current.ComponentID.IsNull() {
				current = channel.Event
			}
		case inbound.FilterActionDrop, inbound.FilterActionQuarantine:
			rejectedEvent := result.Event
			if rejectedEvent.ComponentID.IsNull() {
				rejectedEvent = current
			}
			action := InboundRejectionDrop
			if result.Action == inbound.FilterActionQuarantine {
				action = InboundRejectionQuarantine
			}
			return rejectedEvent, b.reject(rejectedEvent, &channel.Chat, &channel.SourceBinding, action, result.Reason, result.Details...), nil
		default:
			return component.InboundEvent{}, nil, fmt.Errorf("unknown inbound filter action %q", result.Action)
		}
	}
	return current, nil, nil
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
	dropEvent := rejection.Event
	actor := dropEvent.Payload.ResolvedActor()
	details := strings.Join(rejection.Details, " ")
	b.logf(
		"inbound dropped component=%s external_chat=%q external_thread=%q reason=%s actor_id=%q actor_label=%q chat_label=%q preview=%q details=%q",
		dropEvent.ComponentID,
		strings.TrimSpace(dropEvent.Payload.ProviderChatID),
		strings.TrimSpace(dropEvent.Payload.ProviderThreadID),
		rejection.Reason,
		strings.TrimSpace(actor.ID),
		strings.TrimSpace(actor.Label),
		strings.TrimSpace(dropEvent.Payload.ChatLabel),
		inboundPreview(dropEvent.Payload.Text.Text),
		details,
	)
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
	externalChatID := strings.TrimSpace(event.Payload.ProviderChatID)
	if externalChatID == "" {
		return fmt.Errorf("missing inbound provider chat id")
	}
	storage := b.repository()
	if storage == nil {
		return fmt.Errorf("missing inbound drop storage")
	}
	actor := event.Payload.ResolvedActor()
	now := time.Now()

	drop, err := storage.InboundDrops().GetByComponentAndExternalChatID(ctx, event.ComponentID, externalChatID)
	if err != nil {
		return err
	}
	if drop == nil {
		drop = &coremodel.InboundDrop{
			ComponentID:    event.ComponentID,
			ExternalChatID: externalChatID,
			FirstSeenAt:    now,
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
		b.logf("inbound init reply failed component=%s external_chat=%q reason=%s err=%v", event.ComponentID, strings.TrimSpace(event.Payload.ProviderChatID), rejection.Reason, err)
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
	externalChatID := strings.TrimSpace(event.Payload.ProviderChatID)
	externalThreadID := strings.TrimSpace(event.Payload.ProviderThreadID)
	label := strings.TrimSpace(event.Payload.ChatLabel)

	status := "chat is not enabled"
	if rejection.Chat == nil {
		status = "chat is not bound"
	}

	lines := []string{
		status,
		"component: " + componentRef,
		"external_chat_id: " + externalChatID,
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
		bind := "ctgbot chat bind " + componentRef + " " + externalChatID
		if label != "" {
			bind += " " + strconv.Quote(label)
		}
		lines = append(lines, bind)
	}

	return relay.Send(ctx, message.OutboundPayload{
		ProviderChatID:   externalChatID,
		ProviderThreadID: externalThreadID,
		Text:             message.TextMessage{Text: strings.Join(lines, "\n")},
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
