package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bartdeboer/ctgbot/internal/message"
)

var chatActionRefreshInterval = 4 * time.Second

func (c *Component) Send(ctx context.Context, payload message.OutboundPayload) error {
	if payload.IsZero() {
		return nil
	}
	if c == nil || c.api == nil {
		return fmt.Errorf("missing telegram api")
	}
	chatID, err := strconv.ParseInt(strings.TrimSpace(payload.ProviderChannelID), 10, 64)
	if err != nil {
		return fmt.Errorf("parse telegram chat id: %w", err)
	}
	threadID, err := parseTelegramProviderThreadID(payload.ProviderThreadID)
	if err != nil {
		return err
	}
	if len(payload.Attachments) == 0 {
		text := cleanTextForTelegram(payload.Text.Text)
		return c.sendRenderedText(ctx, chatID, threadID, text)
	}
	for i, attachment := range payload.Attachments {
		caption := ""
		if i == 0 {
			caption = payload.Text.Text
		}
		if err := c.sendAttachment(ctx, chatID, threadID, caption, attachment); err != nil {
			return err
		}
	}
	return nil
}

func (c *Component) StartChatAction(ctx context.Context, target message.ChatTarget, action message.ChatAction) (func(), error) {
	if c == nil || c.api == nil {
		return nil, fmt.Errorf("missing telegram api")
	}
	chatID, err := strconv.ParseInt(strings.TrimSpace(target.ProviderChannelID), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse telegram chat id: %w", err)
	}
	threadID, err := parseTelegramProviderThreadID(target.ProviderThreadID)
	if err != nil {
		return nil, err
	}
	if err := c.api.SendChatAction(ctx, chatID, threadID, action); err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	ticker := time.NewTicker(chatActionRefreshInterval)
	var once sync.Once
	stop := func() {
		once.Do(func() {
			ticker.Stop()
			cancel()
		})
	}

	go func() {
		defer stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.api.SendChatAction(runCtx, chatID, threadID, action); err != nil {
					c.logf("telegram chat action failed chat=%d thread=%d action=%q err=%v", chatID, threadID, action, err)
					return
				}
			}
		}
	}()

	return stop, nil
}

func parseTelegramProviderThreadID(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	threadID, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parse telegram thread id: %w", err)
	}
	return threadID, nil
}
