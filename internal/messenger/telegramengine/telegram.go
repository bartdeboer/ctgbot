package telegramengine

import (
	"context"
	"time"

	"github.com/bartdeboer/ctgbot/internal/chatmodel"
	"github.com/bartdeboer/ctgbot/internal/messenger"
)

type TelegramAPI interface {
	Run(ctx context.Context, pollTimeout time.Duration, onUpdate func(ctx context.Context, u chatmodel.TelegramUpdate)) error
	SendMessage(ctx context.Context, chatID int64, threadID int, replyTo int, text string, parseMode string) error
	SendDocument(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error
	SendPhoto(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error
	SendVideo(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error
	SendAudio(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error
	SendChatAction(ctx context.Context, chatID int64, threadID int, action messenger.ChatAction) error
	DownloadFile(ctx context.Context, fileID string) ([]byte, error)
}
