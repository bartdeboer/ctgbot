package app

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/inbound"
	"github.com/bartdeboer/ctgbot/internal/message"
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
