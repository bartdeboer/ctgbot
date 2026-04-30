// Package broker sketches the v2 routing layer.
package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v2/component"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v2/repository"
)

type Broker struct {
	storage    repository.Storage
	components *component.Registry
}

type EventOutcome struct {
	Inbound  *coremodel.ThreadMessage
	Outbound []coremodel.ThreadMessage
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
	chat, thread, err := b.ensureTarget(ctx, event)
	if err != nil {
		return EventOutcome{}, err
	}

	inbound, err := b.appendInbound(ctx, event, chat.ID, thread.ID)
	if err != nil {
		return EventOutcome{}, err
	}

	outbound, err := b.runAgents(ctx, *inbound)
	if err != nil {
		return EventOutcome{Inbound: inbound}, err
	}
	for i := range outbound {
		if err := b.appendAndRelayOutbound(ctx, &outbound[i], *inbound); err != nil {
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
	chat, thread, err := b.ensureTarget(ctx, event)
	if err != nil {
		return nil, err
	}
	return b.appendInbound(ctx, event, chat.ID, thread.ID)
}

func (b *Broker) ensureReady() error {
	if b == nil || b.storage == nil {
		return fmt.Errorf("missing broker storage")
	}
	return nil
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

func (b *Broker) ensureTarget(ctx context.Context, event component.InboundEvent) (*coremodel.Chat, *coremodel.Thread, error) {
	chat, err := b.resolveChat(ctx, event)
	if err != nil {
		return nil, nil, err
	}
	thread, err := b.resolveThread(ctx, event, chat.ID)
	if err != nil {
		return nil, nil, err
	}
	return chat, thread, nil
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

func (b *Broker) runAgents(ctx context.Context, inbound coremodel.ThreadMessage) ([]coremodel.ThreadMessage, error) {
	if b.components == nil {
		return nil, nil
	}

	var outbound []coremodel.ThreadMessage
	for _, agent := range b.components.Agents() {
		message, err := agent.HandleMessage(ctx, inbound)
		if err != nil {
			return outbound, err
		}
		if message == nil || message.Text == "" {
			continue
		}
		outbound = append(outbound, *message)
	}
	return outbound, nil
}

func (b *Broker) appendAndRelayOutbound(ctx context.Context, message *coremodel.ThreadMessage, inbound coremodel.ThreadMessage) error {
	message.ChatID = inbound.ChatID
	message.ThreadID = inbound.ThreadID
	message.Direction = coremodel.DirectionOutbound
	if message.Kind == "" {
		message.Kind = coremodel.MessageKindAgent
	}
	if err := b.storage.Messages().Append(ctx, message); err != nil {
		return err
	}
	return b.relayOutbound(ctx, *message)
}

func (b *Broker) relayOutbound(ctx context.Context, message coremodel.ThreadMessage) error {
	if b.components == nil {
		return nil
	}
	for _, relay := range b.components.OutboundRelays() {
		if err := relay.SendMessage(ctx, message); err != nil {
			return err
		}
	}
	return nil
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
