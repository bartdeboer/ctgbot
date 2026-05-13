package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

func storeInboundMessageWithStorage(ctx context.Context, storage repository.Storage, inbound component.ResolvedInbound) (*coremodel.ThreadMessage, error) {
	if storage == nil {
		return nil, fmt.Errorf("missing message storage")
	}
	actor := inbound.Payload.ResolvedActor()
	metadata := inboundMetadataJSON(inbound.Payload)
	if len(inbound.Metadata) > 0 {
		if metadata != "" {
			metadata += "\n"
		}
		metadata += strings.Join(inbound.Metadata, "\n")
	}
	message := &coremodel.ThreadMessage{
		ChatID:       inbound.Chat.ID,
		ThreadID:     inbound.Thread.ID,
		Direction:    coremodel.MessageDirectionInbound,
		Kind:         inboundKind(inbound),
		ComponentID:  inbound.ComponentID,
		ExternalID:   strings.TrimSpace(inbound.ExternalID),
		ActorLabel:   strings.TrimSpace(actor.Label),
		Text:         strings.TrimSpace(inbound.Payload.Text.Text),
		MetadataJSON: metadata,
	}
	if strings.TrimSpace(actor.ID) != "" {
		message.ActorID = strings.TrimSpace(actor.ID)
	}
	if err := storage.Messages().Append(ctx, message); err != nil {
		return nil, err
	}
	for _, media := range inbound.Payload.Attachments {
		artifact := &coremodel.Artifact{
			ChatID:      inbound.Chat.ID,
			ThreadID:    inbound.Thread.ID,
			MessageID:   message.ID,
			ComponentID: inbound.ComponentID,
			Filename:    strings.TrimSpace(media.Filename),
			ContentType: strings.TrimSpace(media.ContentType),
			Syntax:      strings.TrimSpace(media.Syntax),
			Content:     append([]byte(nil), media.Content...),
		}
		if err := storage.Artifacts().Append(ctx, artifact); err != nil {
			return nil, err
		}
	}
	return message, nil
}

func storeOutboundMessageWithStorage(ctx context.Context, storage repository.Storage, message *coremodel.ThreadMessage, attachments []message.Media) error {
	if storage == nil {
		return fmt.Errorf("missing message storage")
	}
	if message == nil {
		return fmt.Errorf("missing message")
	}
	if err := storage.Messages().Append(ctx, message); err != nil {
		return err
	}
	return storeMessageArtifacts(ctx, storage, message, attachments)
}

func inboundKind(inbound component.ResolvedInbound) coremodel.MessageKind {
	if strings.TrimSpace(inbound.Payload.Text.Text) != "" {
		return coremodel.MessageKindUser
	}
	return coremodel.MessageKindEvent
}

func storeMessageArtifacts(ctx context.Context, storage repository.Storage, message *coremodel.ThreadMessage, attachments []message.Media) error {
	for _, media := range attachments {
		if err := storage.Artifacts().Append(ctx, &coremodel.Artifact{
			ChatID:      message.ChatID,
			ThreadID:    message.ThreadID,
			MessageID:   message.ID,
			ComponentID: message.ComponentID,
			Filename:    strings.TrimSpace(media.Filename),
			ContentType: strings.TrimSpace(media.ContentType),
			Syntax:      strings.TrimSpace(media.Syntax),
			Content:     append([]byte(nil), media.Content...),
		}); err != nil {
			return err
		}
	}
	return nil
}

func inboundMetadataJSON(payload message.InboundPayload) string {
	actor := payload.ResolvedActor()
	var metadata []string
	if payload.ProviderType != "" {
		metadata = append(metadata, "provider="+strings.TrimSpace(payload.ProviderType))
	}
	if payload.ProviderChannelID != "" {
		metadata = append(metadata, "channel="+strings.TrimSpace(payload.ProviderChannelID))
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
