package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v2/component"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

// HandleEvent is the v2 broker happy path:
//
//   - resolve the canonical chat/thread
//   - store the inbound thread message
//   - let agent components produce outbound messages
//   - store and relay outbound messages
func (b *Broker) HandleEvent(ctx context.Context, event component.InboundEvent) (EventOutcome, error) {
	if err := b.ensureReady(); err != nil {
		return EventOutcome{}, err
	}
	if err := validateEvent(event); err != nil {
		return EventOutcome{}, err
	}
	b.logf("v2 inbound event source=%s provider_chat=%q provider_thread=%q external=%q text_len=%d", event.SourceType, event.ProviderChatID, event.ProviderThreadID, event.ExternalID, len(event.Text))
	chat, err := b.resolveChat(ctx, event)
	if err != nil {
		return EventOutcome{}, err
	}
	b.logf("v2 chat resolved chat=%s enabled=%t", chat.ID, chat.Enabled)
	if err := b.ensureSourceChatComponent(ctx, chat.ID, event); err != nil {
		return EventOutcome{}, err
	}
	if !chat.Enabled {
		b.logf("v2 chat disabled chat=%s provider=%s:%s external=%q; dropping event", chat.ID, chat.ProviderType, chat.ProviderChatID, event.ExternalID)
		return EventOutcome{Blocked: true}, nil
	}
	thread, err := b.resolveThread(ctx, event, chat.ID)
	if err != nil {
		return EventOutcome{}, err
	}
	b.logf("v2 thread resolved chat=%s thread=%s", chat.ID, thread.ID)
	runtime, err := b.runtimeForChat(ctx, chat.ID)
	if err != nil {
		return EventOutcome{}, err
	}
	b.logf("v2 chat runtime chat=%s components=%d agents=%d relays=%d", chat.ID, len(runtime.Components), len(runtime.Agents), len(runtime.Relays))

	if handled, outcome, err := b.tryHandleCommand(ctx, event, *chat, *thread, runtime); handled || err != nil {
		return outcome, err
	}

	inbound, err := b.appendInbound(ctx, event, chat.ID, thread.ID)
	if err != nil {
		return EventOutcome{}, err
	}
	b.logf("v2 inbound stored message=%s", inbound.ID)

	outbound, err := b.runAgents(ctx, *inbound, runtime)
	if err != nil {
		return EventOutcome{Inbound: inbound}, err
	}
	for i := range outbound {
		if err := b.appendAndRelayOutbound(ctx, &outbound[i], *inbound, runtime); err != nil {
			return EventOutcome{Inbound: inbound, Outbound: outbound[:i]}, err
		}
	}

	return EventOutcome{Inbound: inbound, Outbound: outbound}, nil
}

// RouteInboundEvent records an external component event as a canonical inbound
// thread message. Agent execution and outbound delivery are deliberately left
// to HandleEvent.
func (b *Broker) RouteInboundEvent(ctx context.Context, event component.InboundEvent) (*coremodel.ThreadMessage, error) {
	if err := b.ensureReady(); err != nil {
		return nil, err
	}
	if err := validateEvent(event); err != nil {
		return nil, err
	}
	chat, err := b.resolveChat(ctx, event)
	if err != nil {
		return nil, err
	}
	if err := b.ensureSourceChatComponent(ctx, chat.ID, event); err != nil {
		return nil, err
	}
	if !chat.Enabled {
		b.logf("v2 chat disabled chat=%s provider=%s:%s external=%q; dropping event", chat.ID, chat.ProviderType, chat.ProviderChatID, event.ExternalID)
		return nil, nil
	}
	thread, err := b.resolveThread(ctx, event, chat.ID)
	if err != nil {
		return nil, err
	}
	return b.appendInbound(ctx, event, chat.ID, thread.ID)
}

func validateEvent(event component.InboundEvent) error {
	if event.ChatID.IsNull() && strings.TrimSpace(event.ProviderChatID) == "" {
		return fmt.Errorf("missing chat id")
	}
	if event.ThreadID.IsNull() && strings.TrimSpace(event.ProviderThreadID) == "" {
		return fmt.Errorf("missing thread id")
	}
	return nil
}

func (b *Broker) resolveChat(ctx context.Context, event component.InboundEvent) (*coremodel.Chat, error) {
	if !event.ChatID.IsNull() {
		chat, err := b.storage.Chats().GetByID(ctx, event.ChatID)
		if err != nil || chat != nil {
			return chat, err
		}
		chat = &coremodel.Chat{
			ID:             event.ChatID,
			ProviderType:   strings.TrimSpace(event.SourceType),
			ProviderChatID: strings.TrimSpace(event.ProviderChatID),
		}
		return chat, b.storage.Chats().Save(ctx, chat)
	}
	return b.storage.Chats().EnsureProviderChat(ctx, event.SourceType, event.ProviderChatID)
}

func (b *Broker) resolveThread(ctx context.Context, event component.InboundEvent, chatID modeluuid.UUID) (*coremodel.Thread, error) {
	if !event.ThreadID.IsNull() {
		thread, err := b.storage.Threads().GetByID(ctx, event.ThreadID)
		if err != nil || thread != nil {
			return thread, err
		}
		thread = &coremodel.Thread{
			ID:               event.ThreadID,
			ChatID:           chatID,
			ProviderThreadID: strings.TrimSpace(event.ProviderThreadID),
		}
		return thread, b.storage.Threads().Save(ctx, thread)
	}
	return b.storage.Threads().EnsureProviderThread(ctx, chatID, event.ProviderThreadID)
}

func (b *Broker) appendInbound(ctx context.Context, event component.InboundEvent, chatID modeluuid.UUID, threadID modeluuid.UUID) (*coremodel.ThreadMessage, error) {
	message := &coremodel.ThreadMessage{
		ChatID:       chatID,
		ThreadID:     threadID,
		Direction:    coremodel.DirectionInbound,
		Kind:         inboundKind(event),
		SourceType:   event.SourceType,
		ExternalID:   event.ExternalID,
		ActorID:      event.Actor.ID,
		ActorLabel:   event.Actor.Label,
		Text:         event.Text,
		MetadataJSON: metadataJSON(event.Metadata),
	}
	if err := b.storage.Messages().Append(ctx, message); err != nil {
		return nil, err
	}
	return message, nil
}

func inboundKind(event component.InboundEvent) coremodel.MessageKind {
	if event.EventType != "" && event.EventType != "message.received" {
		return coremodel.MessageKindEvent
	}
	return coremodel.MessageKindUser
}

func metadataJSON(metadata map[string]string) string {
	if len(metadata) == 0 {
		return ""
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		return ""
	}
	return string(body)
}
