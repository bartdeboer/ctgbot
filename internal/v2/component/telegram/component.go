// Package telegram contains the component-model Telegram implementation.
//
// This package intentionally stays small. It adapts the lower-level Telegram API
// to the v2 component capabilities without pulling in the old broker shape.
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/dbmodel"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/ctgbot/internal/v2/component"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

const (
	ComponentType        = "telegram"
	DefaultProfileName   = "default"
	EventMessageReceived = "message.received"
)

type API interface {
	Run(ctx context.Context, pollTimeout time.Duration, onUpdate func(context.Context, dbmodel.TelegramUpdate)) error
	SendMessage(ctx context.Context, chatID int64, threadID int, replyTo int, text string, parseMode string) error
}

type Component struct {
	API               API
	PollTimeout       time.Duration
	RootUserIDs       []int64
	EventErrorHandler func(context.Context, component.InboundEvent, error)
	Logf              func(format string, args ...any)
}

var _ component.Component = (*Component)(nil)
var _ component.EventSource = (*Component)(nil)
var _ component.OutboundRelay = (*Component)(nil)
var _ component.Profiled = (*Component)(nil)

func New(api API) *Component {
	return &Component{API: api}
}

func (c *Component) Type() string {
	return ComponentType
}

func (c *Component) ProfileName() string {
	return DefaultProfileName
}

func (c *Component) RunEvents(ctx context.Context, emit component.InboundEventEmitter) error {
	if c == nil || c.API == nil {
		return fmt.Errorf("missing telegram api")
	}
	if emit == nil {
		return fmt.Errorf("missing inbound event emitter")
	}
	return c.API.Run(ctx, c.PollTimeout, func(updateCtx context.Context, update dbmodel.TelegramUpdate) {
		event := c.inboundEventFromUpdate(update)
		if strings.TrimSpace(event.Text) == "" {
			c.logf("v2 telegram empty update skipped chat=%d thread=%d msg=%d user=%q user_id=%d attachments=%d", update.ChatID, update.ThreadID, update.MessageID, update.UserLabel(), update.UserID, len(update.Attachments))
			return
		}
		if err := emit(updateCtx, event); err != nil {
			c.logf("v2 telegram event failed chat=%d thread=%d msg=%d err=%v", update.ChatID, update.ThreadID, update.MessageID, err)
			if c.EventErrorHandler != nil {
				c.EventErrorHandler(updateCtx, event, err)
			}
		}
	})
}

func (c *Component) logf(format string, args ...any) {
	if c != nil && c.Logf != nil {
		c.Logf(format, args...)
	}
}

func (c *Component) SendMessage(ctx context.Context, message coremodel.ThreadMessage) error {
	if c == nil || c.API == nil {
		return fmt.Errorf("missing telegram api")
	}
	text := strings.TrimSpace(message.Text)
	if text == "" {
		return nil
	}
	chatID, threadID, err := providerIDsFromMessage(message)
	if err != nil {
		return err
	}
	if err := c.API.SendMessage(ctx, chatID, threadID, 0, text, ""); err != nil {
		return fmt.Errorf("telegram send chat=%d thread=%d: %w", chatID, threadID, err)
	}
	return nil
}

func InboundEventFromUpdate(update dbmodel.TelegramUpdate) component.InboundEvent {
	return inboundEventFromUpdate(update, nil)
}

func (c *Component) inboundEventFromUpdate(update dbmodel.TelegramUpdate) component.InboundEvent {
	return inboundEventFromUpdate(update, c.rolesForUpdate(update))
}

func inboundEventFromUpdate(update dbmodel.TelegramUpdate, roles []simplerbac.Role) component.InboundEvent {
	metadata := map[string]string{
		"telegram.chat_id":    strconv.FormatInt(update.ChatID, 10),
		"telegram.thread_id":  strconv.Itoa(update.ThreadID),
		"telegram.message_id": strconv.Itoa(update.MessageID),
	}
	return component.InboundEvent{
		SourceType:        ComponentType,
		SourceProfileName: DefaultProfileName,
		EventType:         EventMessageReceived,
		ExternalID:        externalID(update.ChatID, update.ThreadID, update.MessageID),
		ProviderChatID:    strconv.FormatInt(update.ChatID, 10),
		ProviderThreadID:  strconv.Itoa(update.ThreadID),
		Actor:             actorFromUpdate(update, roles),
		Text:              strings.TrimSpace(update.Text),
		Metadata:          metadata,
	}
}

func actorFromUpdate(update dbmodel.TelegramUpdate, roles []simplerbac.Role) component.Actor {
	return component.Actor{
		ID:    strconv.FormatInt(update.UserID, 10),
		Label: strings.TrimSpace(update.UserLabel()),
		Roles: append([]simplerbac.Role(nil), roles...),
	}
}

func (c *Component) rolesForUpdate(update dbmodel.TelegramUpdate) []simplerbac.Role {
	roles := []simplerbac.Role{simplerbac.RoleUser}
	for _, userID := range c.RootUserIDs {
		if userID == update.UserID {
			return append(roles, simplerbac.RoleRoot)
		}
	}
	return roles
}

func providerIDsFromMessage(message coremodel.ThreadMessage) (int64, int, error) {
	var metadata map[string]string
	if strings.TrimSpace(message.MetadataJSON) != "" {
		if err := json.Unmarshal([]byte(message.MetadataJSON), &metadata); err != nil {
			return 0, 0, fmt.Errorf("parse telegram metadata: %w", err)
		}
	}
	chatID, err := strconv.ParseInt(strings.TrimSpace(metadata["telegram.chat_id"]), 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("missing telegram chat id")
	}
	threadID := 0
	if rawThreadID := strings.TrimSpace(metadata["telegram.thread_id"]); rawThreadID != "" {
		threadID, err = strconv.Atoi(rawThreadID)
		if err != nil {
			return 0, 0, fmt.Errorf("parse telegram thread id: %w", err)
		}
	}
	return chatID, threadID, nil
}

func externalID(chatID int64, threadID int, messageID int) string {
	return fmt.Sprintf("%d:%d:%d", chatID, threadID, messageID)
}
