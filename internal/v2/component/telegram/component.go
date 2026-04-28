// Package telegram contains the component-model Telegram implementation.
//
// This package intentionally does not wrap telegramengine.TelegramBot. It starts
// from the component event-source model and can grow independently while the
// existing Telegram engine remains in production.
package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/v2/component"
)

const (
	ComponentType        = "telegram"
	EventMessageReceived = "message.received"
)

type API interface {
	Run(ctx context.Context, onUpdate func(context.Context, Update) error) error
}

type Component struct {
	API API
}

var _ component.Component = (*Component)(nil)
var _ component.EventSource = (*Component)(nil)

func New(api API) *Component {
	return &Component{API: api}
}

func (c *Component) Type() string {
	return ComponentType
}

func (c *Component) RunEvents(ctx context.Context, emit component.InboundEventEmitter) error {
	if c == nil || c.API == nil {
		return fmt.Errorf("missing telegram api")
	}
	if emit == nil {
		return fmt.Errorf("missing inbound event emitter")
	}
	return c.API.Run(ctx, func(updateCtx context.Context, update Update) error {
		return emit(updateCtx, update.InboundEvent())
	})
}

type Update struct {
	ChatID    int64
	ThreadID  int
	MessageID int
	UserID    int64
	UserLabel string
	IsAdmin   bool
	Text      string
	Metadata  map[string]string
}

func (u Update) InboundEvent() component.InboundEvent {
	metadata := map[string]string{
		"telegram.chat_id":    strconv.FormatInt(u.ChatID, 10),
		"telegram.thread_id":  strconv.Itoa(u.ThreadID),
		"telegram.message_id": strconv.Itoa(u.MessageID),
	}
	for key, value := range u.Metadata {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		metadata[key] = value
	}
	return component.InboundEvent{
		SourceType: ComponentType,
		EventType:  EventMessageReceived,
		ExternalID: externalID(u),
		Actor: component.Actor{
			ID:      strconv.FormatInt(u.UserID, 10),
			Label:   strings.TrimSpace(u.UserLabel),
			IsAdmin: u.IsAdmin,
		},
		Text:     strings.TrimSpace(u.Text),
		Metadata: metadata,
	}
}

func externalID(u Update) string {
	return fmt.Sprintf("%d:%d:%d", u.ChatID, u.ThreadID, u.MessageID)
}
