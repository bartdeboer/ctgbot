package monitor

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/chatbroker"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type threadLookup interface {
	FindThreadByID(ctx context.Context, threadID modeluuid.UUID) (*chatbroker.Thread, error)
}

type telegramDocumentSender interface {
	SendDocument(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error
}

type FileDeliveryService struct {
	Config   *appstate.Config
	Sessions threadLookup
	Telegram telegramDocumentSender
}

func NewFileDeliveryService(cfg *appstate.Config, sessions threadLookup, telegram telegramDocumentSender) *FileDeliveryService {
	return &FileDeliveryService{
		Config:   cfg,
		Sessions: sessions,
		Telegram: telegram,
	}
}

func (s *FileDeliveryService) SendFile(ctx context.Context, req hostbridge.SendFileRequest) error {
	if s == nil || s.Config == nil {
		return fmt.Errorf("missing config")
	}
	if s.Sessions == nil {
		return fmt.Errorf("missing session store")
	}
	if s.Telegram == nil {
		return fmt.Errorf("missing telegram sender")
	}

	chatID, err := modeluuid.Parse(strings.TrimSpace(req.ChatID))
	if err != nil {
		return fmt.Errorf("parse chat id: %w", err)
	}
	threadID, err := modeluuid.Parse(strings.TrimSpace(req.ThreadID))
	if err != nil {
		return fmt.Errorf("parse thread id: %w", err)
	}

	thread, err := s.Sessions.FindThreadByID(ctx, threadID)
	if err != nil {
		return fmt.Errorf("find thread: %w", err)
	}
	if thread == nil {
		return fmt.Errorf("thread not found: %s", threadID)
	}
	if thread.ChatID != chatID {
		return fmt.Errorf("thread %s does not belong to chat %s", threadID, chatID)
	}

	chatCfg, err := s.Config.FindChatByID(chatID)
	if err != nil {
		return fmt.Errorf("find chat: %w", err)
	}
	if chatCfg == nil {
		return fmt.Errorf("chat not found: %s", chatID)
	}
	if chatCfg.ProviderType != "telegram" {
		return fmt.Errorf("file upload only supported for telegram chats")
	}

	providerChatID, err := strconv.ParseInt(strings.TrimSpace(chatCfg.ProviderChatID), 10, 64)
	if err != nil {
		return fmt.Errorf("parse telegram chat id: %w", err)
	}
	providerThreadID := 0
	if raw := strings.TrimSpace(thread.ProviderThreadID); raw != "" {
		providerThreadID, err = strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("parse telegram thread id: %w", err)
		}
	}

	return s.Telegram.SendDocument(ctx, providerChatID, providerThreadID, req.Filename, req.Caption, req.Content)
}
