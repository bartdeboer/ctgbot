package telegramengine

import (
	"context"
	"time"

	"github.com/bartdeboer/ctgbot/internal/chatmodel"
)

type TelegramAPI interface {
	Run(ctx context.Context, pollTimeout time.Duration, onUpdate func(ctx context.Context, u chatmodel.TelegramUpdate)) error
	SendMessage(ctx context.Context, chatID int64, threadID int, replyTo int, text string) error
}
