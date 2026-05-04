package chatbroker

import (
	"context"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type turnOutputHandler struct {
	broker   *Broker
	threadID modeluuid.UUID

	lastText string
}

func newTurnOutputHandler(broker *Broker, threadID modeluuid.UUID) *turnOutputHandler {
	return &turnOutputHandler{
		broker:   broker,
		threadID: threadID,
	}
}

func (h *turnOutputHandler) Send(ctx context.Context, payload messenger.OutboundPayload) error {
	if h == nil || h.broker == nil {
		return nil
	}

	payload.Text.Text = strings.TrimSpace(payload.Text.Text)
	if payload.IsZero() {
		return nil
	}

	if err := h.broker.SendPayload(ctx, h.threadID, payload); err != nil {
		return err
	}
	h.lastText = payload.Text.Text
	return nil
}

func (h *turnOutputHandler) LastText() string {
	if h == nil {
		return ""
	}
	return h.lastText
}
