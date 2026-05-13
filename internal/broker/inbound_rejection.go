package broker

import (
	"context"
	"strconv"
	"strings"

	component "github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
)

type InboundRejectionAction string

const (
	InboundRejectionDrop       InboundRejectionAction = "drop"
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
