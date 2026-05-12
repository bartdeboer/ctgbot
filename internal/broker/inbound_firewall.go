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
	"github.com/bartdeboer/ctgbot/internal/repository"
)

func inboundFilters(storage repository.Storage, filters ...inbound.Filter) []inbound.Filter {
	out := []inbound.Filter{NewSourceBindingFilter(storage)}
	for _, filter := range filters {
		if filter != nil {
			out = append(out, filter)
		}
	}
	return out
}

func (b *Broker) filterInbound(ctx context.Context, envelope inbound.Envelope) (inbound.Envelope, inbound.FilterResult, error) {
	current := envelope
	for _, filter := range b.InboundFilters {
		if filter == nil {
			continue
		}
		result, err := filter.FilterInbound(ctx, current)
		if err != nil || result.Drop {
			return result.Envelope, result, err
		}
		current = result.Envelope
	}
	return current, inbound.Pass(current), nil
}

type SourceBindingFilter struct {
	Storage repository.Storage
}

func NewSourceBindingFilter(storage repository.Storage) *SourceBindingFilter {
	return &SourceBindingFilter{Storage: storage}
}

func (f *SourceBindingFilter) FilterInbound(ctx context.Context, envelope inbound.Envelope) (inbound.FilterResult, error) {
	event := envelope.Event
	externalChatID := strings.TrimSpace(event.Payload.ProviderChatID)
	if externalChatID == "" {
		return inbound.FilterResult{}, fmt.Errorf("missing inbound provider chat id")
	}
	if f == nil || f.Storage == nil {
		return inbound.FilterResult{}, fmt.Errorf("missing inbound source binding storage")
	}

	sourceBinding, err := f.Storage.ChatComponents().FindByComponentRoleAndExternalChatID(
		ctx,
		event.ComponentID,
		coremodel.ChatComponentRoleSource,
		externalChatID,
	)
	if err != nil {
		return inbound.FilterResult{}, err
	}
	if sourceBinding == nil {
		if err := f.recordInboundDrop(ctx, event); err != nil {
			return inbound.FilterResult{}, err
		}
		return inbound.Drop(envelope, "no-source-binding"), nil
	}
	envelope.SourceBinding = sourceBinding

	chat, err := f.Storage.Chats().GetByID(ctx, sourceBinding.ChatID)
	if err != nil {
		return inbound.FilterResult{}, err
	}
	if chat == nil {
		return inbound.Drop(envelope, "missing-chat"), nil
	}
	envelope.Chat = chat
	if !chat.Enabled {
		return inbound.Drop(envelope, "chat-disabled"), nil
	}
	hasRelay, err := f.hasRelayBinding(ctx, chat.ID)
	if err != nil {
		return inbound.FilterResult{}, err
	}
	if !hasRelay {
		return inbound.Drop(envelope, "no-relay-binding"), nil
	}
	return inbound.Pass(envelope), nil
}

func (f *SourceBindingFilter) hasRelayBinding(ctx context.Context, chatID modeluuid.UUID) (bool, error) {
	if chatID.IsNull() {
		return false, nil
	}
	bindings, err := f.Storage.ChatComponents().ListEnabledByChatID(ctx, chatID)
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

func (f *SourceBindingFilter) recordInboundDrop(ctx context.Context, event component.InboundEvent) error {
	externalChatID := strings.TrimSpace(event.Payload.ProviderChatID)
	if externalChatID == "" {
		return fmt.Errorf("missing inbound provider chat id")
	}
	actor := event.Payload.ResolvedActor()
	now := time.Now()

	drop, err := f.Storage.InboundDrops().GetByComponentAndExternalChatID(ctx, event.ComponentID, externalChatID)
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
	return f.Storage.InboundDrops().Save(ctx, drop)
}

func (b *Broker) maybeHandleInboundFirewallInit(ctx context.Context, result inbound.FilterResult) {
	switch result.Reason {
	case "no-source-binding", "missing-chat", "chat-disabled":
	default:
		return
	}
	event := result.Envelope.Event
	if !isInitCommand(event.Payload.Text.Text) {
		return
	}
	if err := b.sendInboundInitReply(ctx, result); err != nil {
		b.logf("inbound init reply failed component=%s external_chat=%q reason=%s err=%v", event.ComponentID, strings.TrimSpace(event.Payload.ProviderChatID), result.Reason, err)
	}
}

func (b *Broker) sendInboundInitReply(ctx context.Context, result inbound.FilterResult) error {
	resolver := b.resolver()
	if resolver == nil {
		return nil
	}
	event := result.Envelope.Event
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
	if result.Envelope.Chat == nil {
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
	case result.Envelope.Chat != nil:
		lines = append(lines,
			"ctgbot config chat "+result.Envelope.Chat.ID.String()+" set chat.enabled true",
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
