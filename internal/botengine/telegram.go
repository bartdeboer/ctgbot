package botengine

import (
	"context"
	"strings"
	"time"
)

type TelegramUpdate struct {
	ChatID    int64
	ChatTitle string
	MessageID int
	ThreadID  int
	Text      string
	FirstName string
	LastName  string
	Username  string
	UserID    int64
}

func (t *TelegramUpdate) UserLabel() string {
	if t.Username != "" {
		return "@" + t.Username
	}
	return strings.TrimSpace(t.FirstName + " " + t.LastName)
}

type TelegramAPI interface {
	Run(ctx context.Context, pollTimeout time.Duration, onUpdate func(ctx context.Context, u TelegramUpdate)) error
	SendMessage(ctx context.Context, chatID int64, threadID int, replyTo int, text string) error
}
