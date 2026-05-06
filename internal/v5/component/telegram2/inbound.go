package telegram2

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/dbmodel"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	v5component "github.com/bartdeboer/ctgbot/internal/v5/component"
)

func (c *Component) RunInbound(ctx context.Context, emit v5component.InboundEmitter) error {
	if c == nil {
		return fmt.Errorf("missing telegram component")
	}
	if c.api == nil {
		return fmt.Errorf("missing telegram api")
	}
	if c.cfg == nil {
		return fmt.Errorf("missing config")
	}
	if emit == nil {
		return fmt.Errorf("missing inbound emitter")
	}

	handle := func(cbCtx context.Context, update dbmodel.TelegramUpdate) {
		c.handleUpdate(cbCtx, update, emit)
	}
	telegramCfg := c.cfg.Telegram()
	if window := telegramCfg.DebounceWindow(); window > 0 {
		return NewDebouncer(window, c.logger, handle).Run(ctx, c.api, telegramCfg.PollTimeout())
	}
	return c.api.Run(ctx, telegramCfg.PollTimeout(), handle)
}

func (c *Component) handleUpdate(ctx context.Context, update dbmodel.TelegramUpdate, emit v5component.InboundEmitter) {
	text := strings.TrimSpace(update.Text)
	if text == "" && len(update.Attachments) == 0 {
		return
	}

	event := update
	if c.updates != nil {
		if err := c.updates.Create(ctx, &event); err != nil {
			c.logf("persisting telegram event failed (chat=%d msg=%d): %v", update.ChatID, update.MessageID, err)
		}
	}

	eventCtx := context.WithValue(ctx, tgEventKey{}, &event)
	defer c.persistEvent(eventCtx)
	if err := c.emitUpdate(eventCtx, event, text, emit); err != nil {
		c.recordEventError(eventCtx, err)
		c.logf("telegram update handling failed chat=%d thread=%d msg=%d err=%v", update.ChatID, update.ThreadID, update.MessageID, err)
	}
}

func (c *Component) emitUpdate(ctx context.Context, update dbmodel.TelegramUpdate, text string, emit v5component.InboundEmitter) error {
	if emit == nil {
		return fmt.Errorf("missing inbound emitter")
	}
	payload, err := c.inboundPayload(ctx, update, text)
	if err != nil {
		return err
	}
	return emit(ctx, v5component.InboundEvent{
		ComponentID: c.componentID,
		ExternalID:  externalIDForPayload(payload),
		Payload:     payload,
	})
}

func (c *Component) inboundPayload(ctx context.Context, update dbmodel.TelegramUpdate, text string) (messenger.InboundPayload, error) {
	operator := false
	if c.cfg != nil && update.UserID != 0 {
		for _, userID := range c.cfg.Telegram().OperatorUserIDs() {
			if userID == update.UserID {
				operator = true
				break
			}
		}
	}

	payload := messenger.InboundPayload{
		ProviderType:      Type,
		ProviderChatID:    fmt.Sprintf("%d", update.ChatID),
		ProviderThreadID:  fmt.Sprintf("%d", update.ThreadID),
		ProviderMessageID: fmt.Sprintf("%d", update.MessageID),
		ChatLabel:         strings.TrimSpace(update.ChatTitle),
		Actor: messenger.Actor{
			ID:    strconv.FormatInt(update.UserID, 10),
			Label: update.UserLabel(),
			Roles: telegramActorRoles(operator),
		},
		Text: messenger.TextMessage{Text: text},
	}

	c.logf("telegram update chat=%d thread=%d msg=%d user=%q user_id=%d text=%q attachments=%d", update.ChatID, update.ThreadID, update.MessageID, update.UserLabel(), update.UserID, text, len(update.Attachments))

	if len(update.Attachments) > 0 {
		attachments, err := c.loadIncomingAttachments(ctx, update.Attachments)
		if err != nil {
			return messenger.InboundPayload{}, fmt.Errorf("load incoming attachments: %w", err)
		}
		payload.Attachments = attachments
	}
	return payload, nil
}

func externalIDForPayload(payload messenger.InboundPayload) string {
	externalID := strings.TrimSpace(payload.ProviderMessageID)
	if externalID != "" {
		return externalID
	}
	return strings.TrimSpace(payload.ProviderChatID) + ":" + strings.TrimSpace(payload.ProviderThreadID)
}

func telegramActorRoles(operator bool) []simplerbac.Role {
	roles := []simplerbac.Role{simplerbac.RoleUser}
	if operator {
		roles = append(roles, simplerbac.RoleRoot)
	}
	return roles
}
