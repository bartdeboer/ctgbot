package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/inbound"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

const droppedEventRetention = 30 * 24 * time.Hour

func (b *Broker) DropEvent(ctx context.Context, rejection *InboundRejection) (*coremodel.DroppedEvent, error) {
	if rejection == nil {
		return nil, nil
	}
	storage := b.repository()
	if storage == nil {
		return nil, fmt.Errorf("missing dropped event storage")
	}
	event := rejection.Event
	senderKey, senderLabel := inbound.SenderIdentity(event.Payload)
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if _, cleanupErr := storage.DroppedEvents().DeleteExpired(ctx, now); cleanupErr != nil {
		b.logf("dropped event cleanup failed err=%v", cleanupErr)
	}
	dropped := &coremodel.DroppedEvent{
		CreatedAt:         now,
		ExpiresAt:         now.Add(droppedEventRetention),
		Status:            droppedEventStatus(rejection),
		Action:            string(rejection.Action),
		Reason:            strings.TrimSpace(rejection.Reason),
		ChatID:            rejectionChatID(rejection),
		SourceBindingID:   rejectionSourceBindingID(rejection),
		ComponentID:       event.ComponentID,
		ProviderChannelID: strings.TrimSpace(event.Payload.ProviderChannelID),
		ProviderThreadID:  strings.TrimSpace(event.Payload.ProviderThreadID),
		ProviderMessageID: strings.TrimSpace(event.Payload.ProviderMessageID),
		SenderKey:         senderKey,
		SenderLabel:       senderLabel,
		Subject:           inbound.SubjectFromText(event.Payload.Text.Text),
		Preview:           inbound.Preview(event.Payload.Text.Text, 240),
		EventJSON:         string(eventJSON),
	}
	if err := storage.DroppedEvents().Save(ctx, dropped); err != nil {
		return nil, err
	}
	return dropped, nil
}

func droppedEventStatus(rejection *InboundRejection) string {
	if rejection == nil {
		return "dropped"
	}
	if rejection.Action == InboundRejectionQuarantine {
		return "quarantined"
	}
	if strings.TrimSpace(rejection.NoticeText) != "" {
		return "pending"
	}
	return "dropped"
}

func rejectionChatID(rejection *InboundRejection) modeluuid.UUID {
	if rejection == nil || rejection.Chat == nil {
		return modeluuid.Nil
	}
	return rejection.Chat.ID
}

func rejectionSourceBindingID(rejection *InboundRejection) modeluuid.UUID {
	if rejection == nil || rejection.SourceBinding == nil {
		return modeluuid.Nil
	}
	return rejection.SourceBinding.ID
}

func dropIDForNotice(ctx context.Context, storage repository.Storage, drop *coremodel.DroppedEvent) string {
	if drop == nil || drop.ID.IsNull() {
		return ""
	}
	if storage == nil {
		return drop.ID.String()
	}
	ids, err := storage.DroppedEvents().ListIDs(ctx)
	if err != nil {
		return drop.ID.String()
	}
	shortID, err := repository.NewShortIDResolver(ids).ShortIDFor(drop.ID, 6)
	if err != nil {
		return drop.ID.String()
	}
	return shortID
}

func noticeTextForDrop(ctx context.Context, storage repository.Storage, rejection *InboundRejection, drop *coremodel.DroppedEvent) string {
	if rejection == nil {
		return ""
	}
	text := strings.TrimSpace(rejection.NoticeText)
	if text == "" {
		return ""
	}
	return strings.ReplaceAll(text, "{{drop_id}}", dropIDForNotice(ctx, storage, drop))
}

func (b *Broker) sendInboundRejectionNotice(ctx context.Context, rejection *InboundRejection, drop *coremodel.DroppedEvent) error {
	text := noticeTextForDrop(ctx, b.repository(), rejection, drop)
	if text == "" || rejection == nil || rejection.Chat == nil || rejection.SourceBinding == nil {
		return nil
	}
	thread, err := b.Mapper.EnsureThread(ctx, *rejection.SourceBinding, strings.TrimSpace(rejection.Event.Payload.ProviderThreadID))
	if err != nil {
		return err
	}
	runtime, err := b.runtimeForChat(ctx, *rejection.Chat)
	if err != nil {
		return err
	}
	_, err = b.storeAndRelayMessage(ctx, runtime, *rejection.Chat, *thread, coremodel.ThreadMessage{
		Kind:        coremodel.MessageKindSystem,
		ComponentID: rejection.Event.ComponentID,
		ActorID:     "ctgbot",
		ActorLabel:  "ctgbot",
		Text:        text,
	}, "ctgbot")
	return err
}
