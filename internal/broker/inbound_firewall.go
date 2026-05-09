package broker

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	component "github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
)

type inboundFirewallDecision struct {
	Allowed       bool
	Reason        string
	Chat          *coremodel.Chat
	SourceBinding *coremodel.ChatComponent
}

func (b *Broker) checkInboundFirewall(ctx context.Context, event component.InboundEvent) (inboundFirewallDecision, error) {
	externalChatID := strings.TrimSpace(event.Payload.ProviderChatID)
	if externalChatID == "" {
		return inboundFirewallDecision{}, fmt.Errorf("missing inbound provider chat id")
	}

	sourceBinding, err := b.Storage.ChatComponents().FindByComponentRoleAndExternalChatID(
		ctx,
		event.ComponentID,
		coremodel.ChatComponentRoleSource,
		externalChatID,
	)
	if err != nil {
		return inboundFirewallDecision{}, err
	}
	if sourceBinding == nil {
		if err := b.recordInboundDrop(ctx, event); err != nil {
			return inboundFirewallDecision{}, err
		}
		return inboundFirewallDecision{Reason: "no-source-binding"}, nil
	}

	chat, err := b.Storage.Chats().GetByID(ctx, sourceBinding.ChatID)
	if err != nil {
		return inboundFirewallDecision{}, err
	}
	if chat == nil {
		return inboundFirewallDecision{Reason: "missing-chat", SourceBinding: sourceBinding}, nil
	}
	if !chat.Enabled {
		return inboundFirewallDecision{Reason: "chat-disabled", Chat: chat, SourceBinding: sourceBinding}, nil
	}
	return inboundFirewallDecision{
		Allowed:       true,
		Reason:        "allowed",
		Chat:          chat,
		SourceBinding: sourceBinding,
	}, nil
}

func (b *Broker) recordInboundDrop(ctx context.Context, event component.InboundEvent) error {
	if b == nil || b.Storage == nil {
		return fmt.Errorf("missing broker storage")
	}
	externalChatID := strings.TrimSpace(event.Payload.ProviderChatID)
	if externalChatID == "" {
		return fmt.Errorf("missing inbound provider chat id")
	}
	actor := event.Payload.ResolvedActor()
	now := time.Now()

	drop, err := b.Storage.InboundDrops().GetByComponentAndExternalChatID(ctx, event.ComponentID, externalChatID)
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
	return b.Storage.InboundDrops().Save(ctx, drop)
}

func (b *Broker) maybeHandleInboundFirewallInit(ctx context.Context, event component.InboundEvent, decision inboundFirewallDecision) {
	if !isInitCommand(event.Payload.Text.Text) {
		return
	}
	if err := b.sendInboundInitReply(ctx, event, decision); err != nil {
		b.logf("inbound init reply failed component=%s external_chat=%q reason=%s err=%v", event.ComponentID, strings.TrimSpace(event.Payload.ProviderChatID), decision.Reason, err)
	}
}

func (b *Broker) sendInboundInitReply(ctx context.Context, event component.InboundEvent, decision inboundFirewallDecision) error {
	if b == nil || b.Resolver == nil {
		return nil
	}
	loaded, err := b.Resolver.ResolveComponent(ctx, event.ComponentID)
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
	if decision.Chat == nil {
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
	case decision.Chat != nil:
		lines = append(lines,
			"ctgbot config chat "+decision.Chat.ID.String()+" set chat.enabled true",
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
