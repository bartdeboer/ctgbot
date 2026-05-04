package broker

import (
	"context"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	component "github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
)

func (b *Broker) appendInbound(ctx context.Context, chat coremodel.Chat, thread coremodel.Thread, event component.InboundEvent) (*coremodel.ThreadMessage, error) {
	actor := event.Payload.ResolvedActor()
	message := &coremodel.ThreadMessage{
		ChatID:       chat.ID,
		ThreadID:     thread.ID,
		Direction:    coremodel.MessageDirectionInbound,
		Kind:         inboundKind(event),
		ComponentID:  event.ComponentID,
		ExternalID:   strings.TrimSpace(event.ExternalID),
		ActorLabel:   strings.TrimSpace(actor.Label),
		Text:         strings.TrimSpace(event.Payload.Text.Text),
		MetadataJSON: inboundMetadataJSON(event.Payload),
	}
	if strings.TrimSpace(actor.ID) != "" {
		message.ActorID = strings.TrimSpace(actor.ID)
	}
	if err := b.Storage.Messages().Append(ctx, message); err != nil {
		return nil, err
	}
	for _, media := range event.Payload.Attachments {
		artifact := &coremodel.Artifact{
			ChatID:      chat.ID,
			ThreadID:    thread.ID,
			MessageID:   message.ID,
			ComponentID: event.ComponentID,
			Filename:    strings.TrimSpace(media.Filename),
			ContentType: strings.TrimSpace(media.ContentType),
			Syntax:      strings.TrimSpace(media.Syntax),
			Content:     append([]byte(nil), media.Content...),
		}
		if err := b.Storage.Artifacts().Append(ctx, artifact); err != nil {
			return nil, err
		}
	}
	return message, nil
}

func inboundKind(event component.InboundEvent) coremodel.MessageKind {
	if strings.TrimSpace(event.Payload.Text.Text) != "" {
		return coremodel.MessageKindUser
	}
	return coremodel.MessageKindEvent
}

func (b *Broker) appendAndRelayMessage(ctx context.Context, runtime *ChatRuntime, chat coremodel.Chat, thread coremodel.Thread, message coremodel.ThreadMessage, sourceType string) (*coremodel.ThreadMessage, error) {
	message.ChatID = chat.ID
	message.ThreadID = thread.ID
	message.Direction = coremodel.MessageDirectionOutbound
	if message.Kind == "" {
		message.Kind = coremodel.MessageKindAgent
	}
	if strings.TrimSpace(message.ActorLabel) == "" {
		message.ActorLabel = sourceType
	}
	if err := b.Storage.Messages().Append(ctx, &message); err != nil {
		return nil, err
	}
	payload := messenger.OutboundPayload{
		Text: messenger.TextMessage{Text: message.Text},
	}
	targets, err := b.relayTargetsForRuntime(ctx, runtime, thread)
	if err != nil {
		return nil, err
	}
	for _, relay := range runtime.Relays {
		for _, target := range targets {
			outbound := payload
			outbound.ProviderChatID = target.ProviderChatID
			outbound.ProviderThreadID = target.ProviderThreadID
			if err := relay.Send(ctx, outbound); err != nil {
				return nil, err
			}
		}
	}
	return &message, nil
}

func (b *Broker) deliverPayload(ctx context.Context, runtime *ChatRuntime, chat coremodel.Chat, thread coremodel.Thread, payload messenger.OutboundPayload, componentID modeluuid.UUID) ([]coremodel.ThreadMessage, error) {
	if runtime == nil || payload.IsZero() {
		return nil, nil
	}
	message := coremodel.ThreadMessage{
		ChatID:      chat.ID,
		ThreadID:    thread.ID,
		Direction:   coremodel.MessageDirectionOutbound,
		Kind:        coremodel.MessageKindAgent,
		ComponentID: componentID,
		Text:        strings.TrimSpace(payload.Text.Text),
	}
	if err := b.Storage.Messages().Append(ctx, &message); err != nil {
		return nil, err
	}
	for _, media := range payload.Attachments {
		if err := b.Storage.Artifacts().Append(ctx, &coremodel.Artifact{
			ChatID:      chat.ID,
			ThreadID:    thread.ID,
			MessageID:   message.ID,
			ComponentID: componentID,
			Filename:    strings.TrimSpace(media.Filename),
			ContentType: strings.TrimSpace(media.ContentType),
			Syntax:      strings.TrimSpace(media.Syntax),
			Content:     append([]byte(nil), media.Content...),
		}); err != nil {
			return nil, err
		}
	}
	targets, err := b.relayTargetsForRuntime(ctx, runtime, thread)
	if err != nil {
		return nil, err
	}
	for _, relay := range runtime.Relays {
		for _, target := range targets {
			outbound := payload
			outbound.ProviderChatID = target.ProviderChatID
			outbound.ProviderThreadID = target.ProviderThreadID
			if err := relay.Send(ctx, outbound); err != nil {
				return nil, err
			}
		}
	}
	return []coremodel.ThreadMessage{message}, nil
}

func inboundMetadataJSON(payload messenger.InboundPayload) string {
	actor := payload.ResolvedActor()
	var metadata []string
	if payload.ProviderType != "" {
		metadata = append(metadata, "provider="+strings.TrimSpace(payload.ProviderType))
	}
	if payload.ProviderChatID != "" {
		metadata = append(metadata, "chat="+strings.TrimSpace(payload.ProviderChatID))
	}
	if payload.ProviderThreadID != "" {
		metadata = append(metadata, "thread="+strings.TrimSpace(payload.ProviderThreadID))
	}
	if payload.ProviderMessageID != "" {
		metadata = append(metadata, "message="+strings.TrimSpace(payload.ProviderMessageID))
	}
	if strings.TrimSpace(actor.ID) != "" {
		metadata = append(metadata, "actor_id="+strings.TrimSpace(actor.ID))
	}
	if strings.TrimSpace(actor.Label) != "" {
		metadata = append(metadata, "actor_label="+strings.TrimSpace(actor.Label))
	}
	if payload.UserID != 0 {
		metadata = append(metadata, "user_id="+strconv.FormatInt(payload.UserID, 10))
	}
	if len(actor.Roles) > 0 {
		roles := make([]string, 0, len(actor.Roles))
		for _, role := range actor.Roles {
			if strings.TrimSpace(string(role)) == "" {
				continue
			}
			roles = append(roles, strings.TrimSpace(string(role)))
		}
		if len(roles) > 0 {
			metadata = append(metadata, "actor_roles="+strings.Join(roles, ","))
		}
	}
	return strings.Join(metadata, "\n")
}

func (b *Broker) relayTargetsForRuntime(ctx context.Context, runtime *ChatRuntime, thread coremodel.Thread) ([]messenger.ChatTarget, error) {
	if runtime == nil {
		return nil, nil
	}
	var out []messenger.ChatTarget
	for _, binding := range runtime.Bindings {
		if binding.Role != coremodel.ChatComponentRoleRelay {
			continue
		}
		target, ok, err := b.Mapper.RelayTarget(ctx, thread.ID, binding)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		out = append(out, *target)
	}
	return out, nil
}
