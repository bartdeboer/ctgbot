package app

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/inbound"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func (s *service) StoreInboundMessage(ctx context.Context, inbound component.ResolvedInbound) (*coremodel.ThreadMessage, error) {
	return storeInboundMessageWithStorage(ctx, s.Repository(), inbound)
}

func (s *service) StoreOutboundMessage(ctx context.Context, message *coremodel.ThreadMessage, attachments []message.Media) error {
	return storeOutboundMessageWithStorage(ctx, s.Repository(), message, attachments)
}

func (s *service) DropEvent(ctx context.Context, rejection *inbound.Rejection) (*coremodel.DroppedEvent, error) {
	return dropEventWithStorage(ctx, s.Repository(), rejection, s.Logf)
}

func (s *service) DropNoticeID(ctx context.Context, drop *coremodel.DroppedEvent) string {
	return dropNoticeIDWithStorage(ctx, s.Repository(), drop)
}

func (s *service) ResolveDroppedEventID(ctx context.Context, ref string) (modeluuid.UUID, error) {
	return resolveDroppedEventIDWithStorage(ctx, s.Repository(), ref)
}

func (s *service) DroppedEvent(ctx context.Context, id modeluuid.UUID) (*coremodel.DroppedEvent, error) {
	storage := s.Repository()
	if storage == nil {
		return nil, fmt.Errorf("missing dropped event storage")
	}
	return storage.DroppedEvents().GetByID(ctx, id)
}

func (s *service) ListDroppedEvents(ctx context.Context, limit int) ([]coremodel.DroppedEvent, error) {
	return listDroppedEventsWithStorage(ctx, s.Repository(), limit)
}

func (s *service) SaveDroppedEvent(ctx context.Context, drop *coremodel.DroppedEvent) error {
	storage := s.Repository()
	if storage == nil {
		return fmt.Errorf("missing dropped event storage")
	}
	return storage.DroppedEvents().Save(ctx, drop)
}
