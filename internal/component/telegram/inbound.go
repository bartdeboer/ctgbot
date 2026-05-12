package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	componentpkg "github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func (c *Component) RunInbound(ctx context.Context, emit componentpkg.InboundEmitter) error {
	if c == nil {
		return fmt.Errorf("missing telegram component")
	}
	for c.api == nil {
		if err := c.loadAPIFromProfile(); err != nil {
			if !errors.Is(err, errMissingTelegramToken) {
				return err
			}
			c.logf("telegram token missing; waiting for %s in component profile", TokenFilename)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(30 * time.Second):
				continue
			}
		}
	}
	if emit == nil {
		return fmt.Errorf("missing inbound emitter")
	}

	handle := func(cbCtx context.Context, update TelegramUpdate) {
		c.handleUpdate(cbCtx, update, emit)
	}
	config := c.componentConfig.withDefaults()
	if window := config.debounceWindow(); window > 0 {
		return NewDebouncer(window, c.logger, handle).Run(ctx, c.api, config.pollTimeout())
	}
	return c.api.Run(ctx, config.pollTimeout(), handle)
}

func (c *Component) handleUpdate(ctx context.Context, update TelegramUpdate, emit componentpkg.InboundEmitter) {
	text := strings.TrimSpace(update.Text)
	if text == "" && len(update.Attachments) == 0 {
		return
	}

	if err := c.emitUpdate(ctx, update, text, emit); err != nil {
		c.logf("telegram update handling failed chat=%d thread=%d msg=%d err=%v", update.ChatID, update.ThreadID, update.MessageID, err)
	}
}

func (c *Component) emitUpdate(ctx context.Context, update TelegramUpdate, text string, emit componentpkg.InboundEmitter) error {
	if emit == nil {
		return fmt.Errorf("missing inbound emitter")
	}
	payload, err := c.inboundPayload(ctx, update, text)
	if err != nil {
		return err
	}
	return emit(ctx, componentpkg.InboundEvent{
		ComponentID: c.componentID,
		ExternalID:  externalIDForPayload(payload),
		Payload:     payload,
	})
}

func (c *Component) inboundPayload(ctx context.Context, update TelegramUpdate, text string) (message.InboundPayload, error) {
	operator := false
	if update.UserID != 0 {
		for _, userID := range c.componentConfig.withDefaults().Operators {
			if userID == update.UserID {
				operator = true
				break
			}
		}
	}

	payload := message.InboundPayload{
		ProviderType:      Type,
		ProviderChatID:    fmt.Sprintf("%d", update.ChatID),
		ProviderThreadID:  fmt.Sprintf("%d", update.ThreadID),
		ProviderMessageID: fmt.Sprintf("%d", update.MessageID),
		ChatLabel:         strings.TrimSpace(update.ChatTitle),
		Actor: message.Actor{
			ID:    strconv.FormatInt(update.UserID, 10),
			Label: update.UserLabel(),
			Roles: telegramActorRoles(operator),
		},
		Text: message.TextMessage{Text: text},
	}

	c.logf("telegram update chat=%d thread=%d msg=%d user=%q user_id=%d text=%q attachments=%d", update.ChatID, update.ThreadID, update.MessageID, update.UserLabel(), update.UserID, text, len(update.Attachments))

	if len(update.Attachments) > 0 {
		attachments, err := c.loadIncomingAttachments(ctx, update.Attachments)
		if err != nil {
			return message.InboundPayload{}, fmt.Errorf("load incoming attachments: %w", err)
		}
		payload.Attachments = attachments
	}
	return payload, nil
}

func externalIDForPayload(payload message.InboundPayload) string {
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
