package telegramengine

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appconfig"
	"github.com/bartdeboer/ctgbot/internal/chatbroker"
	"github.com/bartdeboer/ctgbot/internal/chatmodel"
)

type tgEventKey struct{}

type TelegramBot struct {
	API     TelegramAPI
	Updates *UpdateStorage
	Broker  *chatbroker.Broker
	Config  *appconfig.Config
	Logger  *log.Logger
}

func NewTelegramBot(api TelegramAPI, updates *UpdateStorage, broker *chatbroker.Broker, cfg *appconfig.Config, logger *log.Logger) *TelegramBot {
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
	if text == "" {
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

	tb.logf("telegram update chat=%d thread=%d msg=%d user=%q text=%q", u.ChatID, u.ThreadID, u.MessageID, u.UserLabel(), text)

	chatLabel := strings.TrimSpace(u.ChatTitle)
	if chatLabel == "" {
		chatLabel = u.UserLabel()
	}
	if err := tb.Config.PersistChatID(u.ChatID, chatLabel); err != nil {
		tb.logf("persisting chatID failed (chat=%d): %v", u.ChatID, err)
	}
	if !tb.Config.ChatEnabled(u.ChatID) {
		tb.logf("ignoring update from disabled chat=%d title=%q", u.ChatID, chatLabel)
		return nil
	}

	if strings.HasPrefix(text, "/") {
		args := normalizeTelegramCommand(text)
		if len(args) == 0 {
			return nil
		}
		reply, err := tb.Broker.HandleCommand(ctx, u.ChatID, u.ThreadID, args[0], args[1:])
		if err != nil {
			tb.recordEventError(ctx, err)
			_ = tb.replyText(ctx, u, fmt.Sprintf("command error: %v", err))
			return nil
		}
		if reply != "" {
			if err := tb.replyText(ctx, u, reply); err != nil {
				return err
			}
		}
		return nil
	}

	if err := tb.handlePrompt(ctx, u, text); err != nil {
		tb.recordEventError(ctx, err)
		_ = tb.replyText(ctx, u, fmt.Sprintf("conversation error: %v", err))
	}
	return nil
}

func (tb *TelegramBot) handlePrompt(ctx context.Context, u chatmodel.TelegramUpdate, prompt string) error {
	outcome, err := tb.Broker.HandlePrompt(ctx, u.ChatID, u.ThreadID, prompt)
	if outcome.Started && outcome.Session != nil {
		msg := fmt.Sprintf("conversation started\ncontainer: %s\nworkspace: %s", outcome.Session.ContainerName, outcome.Session.WorkspaceHost)
		if err := tb.replyText(ctx, u, msg); err != nil {
			return err
		}
	}
	reply := outcome.Reply
	if reply != "" {
		reply = cleanTextForTelegram(reply)
	}
	if reply != "" {
		if err := tb.replyText(ctx, u, reply); err != nil {
			return err
		}
	}
	return err
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

func normalizeTelegramCommand(text string) []string {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return nil
	}
	fields[0] = strings.TrimPrefix(fields[0], "/")
	if i := strings.Index(fields[0], "@"); i >= 0 {
		fields[0] = fields[0][:i]
	}
	return fields
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
