package telegramengine

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bartdeboer/ctgbot/internal/chatmodel"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type TelegramAPIV2 struct {
	token string

	mu sync.RWMutex
	b  *bot.Bot
}

func NewTelegramAPIV2(token string) (*TelegramAPIV2, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("missing telegram token")
	}
	return &TelegramAPIV2{token: token}, nil
}

func (a *TelegramAPIV2) Run(ctx context.Context, pollTimeout time.Duration, onUpdate func(ctx context.Context, u chatmodel.TelegramUpdate)) error {
	if onUpdate == nil {
		return fmt.Errorf("onUpdate is nil")
	}

	opts := []bot.Option{
		bot.WithDefaultHandler(func(hctx context.Context, b *bot.Bot, upd *models.Update) {
			if upd == nil || upd.Message == nil {
				return
			}
			msg := upd.Message
			tupd := chatmodel.TelegramUpdate{
				ChatID:    msg.Chat.ID,
				ChatTitle: msg.Chat.Title,
				ThreadID:  msg.MessageThreadID,
				MessageID: msg.ID,
				Text:      msg.Text,
			}
			if msg.From != nil {
				tupd.FirstName = msg.From.FirstName
				tupd.LastName = msg.From.LastName
				tupd.Username = msg.From.Username
				tupd.UserID = msg.From.ID
			}
			onUpdate(hctx, tupd)
		}),
		bot.WithWorkers(16),
	}

	if pollTimeout > 0 {
		opts = append(opts, bot.WithHTTPClient(pollTimeout, &http.Client{Timeout: pollTimeout}))
	}

	b, err := bot.New(a.token, opts...)
	if err != nil {
		return err
	}

	a.mu.Lock()
	a.b = b
	a.mu.Unlock()

	b.Start(ctx)
	return ctx.Err()
}

func (a *TelegramAPIV2) SendMessage(ctx context.Context, chatID int64, threadID int, replyTo int, text string) error {
	b := a.getBot()
	if b == nil {
		return fmt.Errorf("telegram bot not initialized")
	}

	p := &bot.SendMessageParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		Text:            text,
	}

	_, err := b.SendMessage(ctx, p)
	return err
}

func (a *TelegramAPIV2) SendDocument(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error {
	b := a.getBot()
	if b == nil {
		return fmt.Errorf("telegram bot not initialized")
	}
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return fmt.Errorf("missing filename")
	}

	p := &bot.SendDocumentParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		Document: &models.InputFileUpload{
			Filename: filename,
			Data:     bytes.NewReader(content),
		},
		Caption: strings.TrimSpace(caption),
	}
	_, err := b.SendDocument(ctx, p)
	return err
}

func (a *TelegramAPIV2) getBot() *bot.Bot {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.b
}
