package telegramengine

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/chatbroker"
	"github.com/bartdeboer/ctgbot/internal/chatmodel"
)

type tgEventKey struct{}

type TelegramBot struct {
	API     TelegramAPI
	Updates *UpdateStorage
	Broker  *chatbroker.Broker
	Config  *appstate.Config
	Logger  *log.Logger
}

func NewTelegramBot(api TelegramAPI, updates *UpdateStorage, broker *chatbroker.Broker, cfg *appstate.Config, logger *log.Logger) *TelegramBot {
	return &TelegramBot{
		API:     api,
		Updates: updates,
		Broker:  broker,
		Config:  cfg,
		Logger:  logger,
	}
}

func (tb *TelegramBot) AutoMigrate(ctx context.Context) error {
	if tb.Updates != nil {
		if err := tb.Updates.AutoMigrate(ctx); err != nil {
			return err
		}
	}
	if tb.Broker != nil {
		if err := tb.Broker.AutoMigrate(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (tb *TelegramBot) Run(ctx context.Context) error {
	if tb.Config == nil {
		return fmt.Errorf("missing config")
	}
	if err := tb.Config.EnsurePaths(); err != nil {
		return err
	}
	return tb.API.Run(ctx, tb.Config.PollTimeout(), func(cbCtx context.Context, u chatmodel.TelegramUpdate) {
		tb.handleUpdate(cbCtx, u)
	})
}

func (tb *TelegramBot) handleUpdate(ctx context.Context, u chatmodel.TelegramUpdate) {
	text := strings.TrimSpace(u.Text)
	if text == "" && len(u.Attachments) == 0 {
		return
	}

	event := u
	if tb.Updates != nil {
		if err := tb.Updates.Create(ctx, &event); err != nil {
			tb.logf("persisting telegram event failed (chat=%d msg=%d): %v", u.ChatID, u.MessageID, err)
		}
	}
	ctx = context.WithValue(ctx, tgEventKey{}, &event)
	defer tb.persistEvent(ctx)

	if err := tb.handleUpdateSerialized(ctx, u, text); err != nil {
		tb.recordEventError(ctx, err)
		tb.logf("telegram update handling failed chat=%d thread=%d msg=%d err=%v", u.ChatID, u.ThreadID, u.MessageID, err)
	}
}

func (tb *TelegramBot) handleUpdateSerialized(ctx context.Context, u chatmodel.TelegramUpdate, text string) error {
	msg := chatbroker.IncomingMessage{
		ProviderType:      "telegram",
		ProviderChatID:    fmt.Sprintf("%d", u.ChatID),
		ProviderThreadID:  fmt.Sprintf("%d", u.ThreadID),
		Message:           text,
		ChatLabel:         strings.TrimSpace(u.ChatTitle),
		UserLabel:         u.UserLabel(),
		ProviderMessageID: fmt.Sprintf("%d", u.MessageID),
	}

	tb.logf("telegram update chat=%d thread=%d msg=%d user=%q text=%q attachments=%d", u.ChatID, u.ThreadID, u.MessageID, u.UserLabel(), text, len(u.Attachments))

	if len(u.Attachments) > 0 {
		uploadReplies, err := tb.handleAttachmentUploads(ctx, u, msg)
		if err != nil {
			return fmt.Errorf("handle attachment uploads: %w", err)
		}
		for _, reply := range uploadReplies {
			if strings.TrimSpace(reply) == "" {
				continue
			}
			if err := tb.replyText(ctx, u, reply); err != nil {
				return err
			}
		}
	}

	if text == "" {
		return nil
	}

	result, err := tb.Broker.HandleIncomingMessage(ctx, msg)
	if err != nil {
		return err
	}
	for _, message := range result.Messages {
		if err := tb.replyText(ctx, u, message.Text); err != nil {
			return err
		}
	}
	return nil
}

func (tb *TelegramBot) handleAttachmentUploads(ctx context.Context, u chatmodel.TelegramUpdate, msg chatbroker.IncomingMessage) ([]string, error) {
	if tb.Broker == nil {
		return nil, fmt.Errorf("missing broker")
	}
	if tb.Config == nil {
		return nil, fmt.Errorf("missing config")
	}

	chatCfg, _, err := tb.Broker.ResolveIncomingThread(ctx, msg, true)
	if err != nil {
		return nil, err
	}
	if chatCfg == nil {
		return nil, fmt.Errorf("missing chat mapping")
	}
	if !chatCfg.Enabled {
		tb.logf("ignoring document upload from disabled chat provider=%q chat=%q title=%q", msg.ProviderType, msg.ProviderChatID, chatCfg.ProviderChatID)
		return nil, nil
	}

	workspaceHost, err := tb.Config.ResolveChatWorkspaceHostPathByID(chatCfg.ID, "")
	if err != nil {
		return nil, err
	}
	inboxHost := filepath.Join(workspaceHost, "inbox")
	if err := os.MkdirAll(inboxHost, 0o755); err != nil {
		return nil, err
	}

	replies := make([]string, 0, len(u.Attachments))
	for _, attachment := range u.Attachments {
		content, err := tb.API.DownloadFile(ctx, attachment.FileID)
		if err != nil {
			return nil, err
		}
		filename := safeIncomingFilename(attachment.Filename)
		targetHost := filepath.Join(inboxHost, filename)
		if err := os.WriteFile(targetHost, content, 0o644); err != nil {
			return nil, err
		}
		replies = append(replies, fmt.Sprintf("upload saved: /workspace/inbox/%s", filename))
	}
	return replies, nil
}

func (tb *TelegramBot) replyText(ctx context.Context, u chatmodel.TelegramUpdate, text string) error {
	text = cleanTextForTelegram(text)
	if text == "" {
		text = "(empty response)"
	}
	tb.appendEventResponse(ctx, text)

	for _, chunk := range splitTelegramText(text, 3500) {
		if err := tb.API.SendMessage(ctx, u.ChatID, u.ThreadID, u.MessageID, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (tb *TelegramBot) logf(format string, args ...any) {
	if tb.Logger != nil {
		tb.Logger.Printf(format, args...)
	}
}

func splitTelegramText(text string, limit int) []string {
	if limit <= 0 || len(text) <= limit {
		return []string{text}
	}

	var chunks []string
	for len(text) > limit {
		cut := strings.LastIndex(text[:limit], "\n")
		if cut <= 0 {
			cut = limit
		}
		chunks = append(chunks, strings.TrimSpace(text[:cut]))
		text = strings.TrimSpace(text[cut:])
	}
	if text != "" {
		chunks = append(chunks, text)
	}
	return chunks
}

func cleanTextForTelegram(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.TrimSpace(text)
}

func safeIncomingFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "upload.bin"
	}
	return name
}

func (tb *TelegramBot) appendEventResponse(ctx context.Context, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	event, ok := ctx.Value(tgEventKey{}).(*chatmodel.TelegramUpdate)
	if !ok || event == nil {
		return
	}
	if strings.TrimSpace(event.ResponseText) == "" {
		event.ResponseText = text
		return
	}
	event.ResponseText += "\n\n" + text
}

func (tb *TelegramBot) recordEventError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	event, ok := ctx.Value(tgEventKey{}).(*chatmodel.TelegramUpdate)
	if !ok || event == nil {
		return
	}
	event.ErrorText = err.Error()
}

func (tb *TelegramBot) persistEvent(ctx context.Context) {
	if tb.Updates == nil {
		return
	}
	event, ok := ctx.Value(tgEventKey{}).(*chatmodel.TelegramUpdate)
	if !ok || event == nil || event.ID == 0 {
		return
	}
	if err := tb.Updates.Save(ctx, event); err != nil {
		tb.logf("persisting telegram event result failed (id=%d): %v", event.ID, err)
	}
}
