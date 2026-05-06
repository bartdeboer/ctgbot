package telegram2

import (
	"context"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/dbmodel"
)

type tgEventKey struct{}

func (c *Component) appendEventResponse(ctx context.Context, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	event, ok := ctx.Value(tgEventKey{}).(*dbmodel.TelegramUpdate)
	if !ok || event == nil {
		return
	}
	if strings.TrimSpace(event.ResponseText) == "" {
		event.ResponseText = text
		return
	}
	event.ResponseText += "\n\n" + text
}

func (c *Component) recordEventError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	event, ok := ctx.Value(tgEventKey{}).(*dbmodel.TelegramUpdate)
	if !ok || event == nil {
		return
	}
	event.ErrorText = err.Error()
}

func (c *Component) persistEvent(ctx context.Context) {
	if c == nil || c.updates == nil {
		return
	}
	event, ok := ctx.Value(tgEventKey{}).(*dbmodel.TelegramUpdate)
	if !ok || event == nil || event.ID == 0 {
		return
	}
	if err := c.updates.Save(ctx, event); err != nil {
		c.logf("persisting telegram event result failed (id=%d): %v", event.ID, err)
	}
}
