package broker

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	v3component "github.com/bartdeboer/ctgbot/internal/v3/component"
	"github.com/bartdeboer/ctgbot/internal/v3/coremodel"
)

func (b *Broker) appendInbound(ctx context.Context, chat coremodel.Chat, thread coremodel.Thread, event v3component.InboundEvent) (*coremodel.ThreadMessage, error) {
	message := &coremodel.ThreadMessage{
		ChatID:       chat.ID,
		ThreadID:     thread.ID,
		Direction:    coremodel.MessageDirectionInbound,
		Kind:         inboundKind(event),
		ComponentID:  event.ComponentID,
		ExternalID:   strings.TrimSpace(event.ExternalID),
		ActorLabel:   strings.TrimSpace(event.Payload.UserLabel),
		Text:         strings.TrimSpace(event.Payload.Text.Text),
		MetadataJSON: inboundMetadataJSON(event.Payload),
	}
	if event.Payload.UserID != 0 {
		message.ActorID = fmt.Sprintf("%d", event.Payload.UserID)
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

func inboundKind(event v3component.InboundEvent) coremodel.MessageKind {
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

func (b *Broker) resolveThread(ctx context.Context, sourceBinding coremodel.ChatComponent, externalThreadID string) (*coremodel.Thread, *coremodel.ThreadComponentState, error) {
	state, err := b.Storage.ThreadComponentStates().FindByComponentAndExternalThreadID(ctx, sourceBinding.ComponentID, strings.TrimSpace(externalThreadID))
	if err != nil {
		return nil, nil, err
	}
	if state != nil {
		thread, err := b.Storage.Threads().GetByID(ctx, state.ThreadID)
		return thread, state, err
	}
	thread := &coremodel.Thread{
		ChatID: sourceBinding.ChatID,
	}
	if strings.TrimSpace(externalThreadID) != "" {
		thread.Label = "thread " + strings.TrimSpace(externalThreadID)
	}
	if err := b.Storage.Threads().Save(ctx, thread); err != nil {
		return nil, nil, err
	}
	state = &coremodel.ThreadComponentState{
		ThreadID:         thread.ID,
		ComponentID:      sourceBinding.ComponentID,
		ExternalThreadID: strings.TrimSpace(externalThreadID),
	}
	if err := b.Storage.ThreadComponentStates().Save(ctx, state); err != nil {
		return nil, nil, err
	}
	return thread, state, nil
}

func inboundMetadataJSON(payload messenger.InboundPayload) string {
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
	if payload.UserID != 0 {
		metadata = append(metadata, "user_id="+strconv.FormatInt(payload.UserID, 10))
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
		state, err := b.Storage.ThreadComponentStates().GetByThreadAndComponent(ctx, thread.ID, binding.ComponentID)
		if err != nil {
			return nil, err
		}
		if state == nil {
			continue
		}
		out = append(out, messenger.ChatTarget{
			ProviderChatID:   strings.TrimSpace(binding.ExternalChatID),
			ProviderThreadID: strings.TrimSpace(state.ExternalThreadID),
		})
	}
	return out, nil
}
