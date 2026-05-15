package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/inbound"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

const DroppedEventRetention = 30 * 24 * time.Hour

func dropEventWithStorage(ctx context.Context, storage repository.Storage, rejection *inbound.Rejection, logf func(format string, args ...any)) (*coremodel.DroppedEvent, error) {
	if rejection == nil {
		return nil, nil
	}
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
	if _, cleanupErr := storage.DroppedEvents().DeleteExpired(ctx, now); cleanupErr != nil && logf != nil {
		logf("dropped event cleanup failed err=%v", cleanupErr)
	}
	dropped := &coremodel.DroppedEvent{
		CreatedAt:         now,
		ExpiresAt:         now.Add(DroppedEventRetention),
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

func dropNoticeIDWithStorage(ctx context.Context, storage repository.Storage, drop *coremodel.DroppedEvent) string {
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

func resolveDroppedEventIDWithStorage(ctx context.Context, storage repository.Storage, ref string) (modeluuid.UUID, error) {
	if storage == nil {
		return modeluuid.Nil, fmt.Errorf("missing dropped event storage")
	}
	ids, err := storage.DroppedEvents().ListIDs(ctx)
	if err != nil {
		return modeluuid.Nil, err
	}
	id, err := repository.NewShortIDResolver(ids).Resolve(ref)
	if err == nil {
		return id, nil
	}
	var notFound *repository.ShortIDNotFoundError
	if errors.As(err, &notFound) {
		return modeluuid.Nil, fmt.Errorf("dropped event not found: %s", strings.TrimSpace(ref))
	}
	return modeluuid.Nil, err
}

func listDroppedEventsWithStorage(ctx context.Context, storage repository.Storage, limit int) ([]coremodel.DroppedEvent, error) {
	if storage == nil {
		return nil, fmt.Errorf("missing dropped event storage")
	}
	ids, err := storage.DroppedEvents().ListIDs(ctx)
	if err != nil {
		return nil, err
	}
	events := make([]coremodel.DroppedEvent, 0, len(ids))
	for _, id := range ids {
		drop, err := storage.DroppedEvents().GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if drop != nil {
			events = append(events, *drop)
		}
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].CreatedAt.After(events[j].CreatedAt)
	})
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}
	return events, nil
}

func droppedEventStatus(rejection *inbound.Rejection) string {
	if rejection == nil {
		return "dropped"
	}
	if rejection.Action == inbound.RejectionQuarantine {
		return "quarantined"
	}
	if strings.TrimSpace(rejection.NoticeText) != "" {
		return "pending"
	}
	return "dropped"
}

func rejectionChatID(rejection *inbound.Rejection) modeluuid.UUID {
	if rejection == nil || rejection.Chat == nil {
		return modeluuid.Nil
	}
	return rejection.Chat.ID
}

func rejectionSourceBindingID(rejection *inbound.Rejection) modeluuid.UUID {
	if rejection == nil || rejection.SourceBinding == nil {
		return modeluuid.Nil
	}
	return rejection.SourceBinding.ID
}
