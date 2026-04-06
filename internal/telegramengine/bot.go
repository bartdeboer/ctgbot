package telegramengine

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appconfig"
	"github.com/bartdeboer/ctgbot/internal/chatbroker"
	"github.com/bartdeboer/ctgbot/internal/chatmodel"
	"github.com/bartdeboer/go-clir"
)

type tgUpdateKey struct{}
type tgEventKey struct{}

type ConversationBroker interface {
	AutoMigrate(ctx context.Context) error
	GetActiveConversation(ctx context.Context, chatID int64, threadID int) (*chatbroker.ChatSession, error)
	StartConversation(ctx context.Context, chatID int64, threadID int, workspace string, replace bool) (*chatbroker.ChatSession, error)
	StopConversation(ctx context.Context, chatID int64, threadID int) error
	HandlePrompt(ctx context.Context, chatID int64, threadID int, prompt string) (chatbroker.PromptOutcome, error)
}

type TelegramBot struct {
	API      TelegramAPI
	Updates  *UpdateStorage
	Broker   ConversationBroker
	Dispatch *Dispatcher
	Config   *appconfig.Config
	Logger   *log.Logger
	router   *clir.Router
}

func NewTelegramBot(api TelegramAPI, updates *UpdateStorage, broker ConversationBroker, cfg *appconfig.Config, logger *log.Logger) *TelegramBot {
	dispatcher := NewDispatcher()
	return &TelegramBot{
		API:      api,
		Updates:  updates,
		Broker:   broker,
		Dispatch: dispatcher,
		Config:   cfg,
		Logger:   logger,
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

	tb.router = tb.buildRouter()
	return tb.API.Run(ctx, tb.Config.PollTimeout(), func(cbCtx context.Context, u chatmodel.TelegramUpdate) {
		tb.handleUpdate(cbCtx, u)
	})
}

func (tb *TelegramBot) buildRouter() *clir.Router {
	r := clir.New()

	r.Routes(func(b *clir.Builder) {
		tg := clir.WithContext(b, func(req *clir.Request) (chatmodel.TelegramUpdate, error) {
			v := req.Context().Value(tgUpdateKey{})
			u, ok := v.(chatmodel.TelegramUpdate)
			if !ok {
				return chatmodel.TelegramUpdate{}, fmt.Errorf("missing TelegramUpdate in context")
			}
			return u, nil
		})

		tg.Handle("new", "Start a new Codex conversation", func(req *clir.Request, u chatmodel.TelegramUpdate) error {
			workspace := ""
			if len(req.Extra) > 0 {
				workspace = req.Extra[0]
			}
			return tb.handleNewConversation(req.Context(), u, workspace)
		})

		tg.Handle("stop", "Stop the active Codex conversation", func(req *clir.Request, u chatmodel.TelegramUpdate) error {
			return tb.handleStopConversation(req.Context(), u)
		})

		tg.Handle("status", "Show the active Codex conversation", func(req *clir.Request, u chatmodel.TelegramUpdate) error {
			return tb.handleStatus(req.Context(), u)
		})

		tg.Handle("help", "Show available commands", func(req *clir.Request, u chatmodel.TelegramUpdate) error {
			return tb.replyText(req.Context(), u, "Commands:\n/new [absolute-host-path]\n/status\n/stop\n/help\n\nAny non-command message is sent to the active Codex conversation.")
		})
	})

	return r
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

	key := chatmodel.ChatKey{ChatID: u.ChatID, ThreadID: u.ThreadID}
	if err := tb.Dispatch.Run(ctx, key, func(runCtx context.Context) error {
		return tb.handleUpdateSerialized(runCtx, u, text)
	}); err != nil {
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
		cmdCtx := context.WithValue(ctx, tgUpdateKey{}, u)
		if err := tb.router.Run(cmdCtx, args); err != nil {
			tb.recordEventError(ctx, err)
			_ = tb.replyText(ctx, u, fmt.Sprintf("command error: %v", err))
		}
		return nil
	}

	if err := tb.handlePrompt(ctx, u, text); err != nil {
		tb.recordEventError(ctx, err)
		_ = tb.replyText(ctx, u, fmt.Sprintf("conversation error: %v", err))
	}
	return nil
}

func (tb *TelegramBot) handleNewConversation(ctx context.Context, u chatmodel.TelegramUpdate, workspace string) error {
	conv, err := tb.Broker.StartConversation(ctx, u.ChatID, u.ThreadID, workspace, true)
	if err != nil {
		return err
	}

	msg := fmt.Sprintf("conversation started\ncontainer: %s\nworkspace: %s", conv.ContainerName, conv.WorkspaceHost)
	return tb.replyText(ctx, u, msg)
}

func (tb *TelegramBot) handleStopConversation(ctx context.Context, u chatmodel.TelegramUpdate) error {
	conv, err := tb.Broker.GetActiveConversation(ctx, u.ChatID, u.ThreadID)
	if err != nil {
		return err
	}
	if conv == nil {
		return tb.replyText(ctx, u, "no active conversation")
	}
	if err := tb.Broker.StopConversation(ctx, u.ChatID, u.ThreadID); err != nil {
		return err
	}
	return tb.replyText(ctx, u, "conversation stopped")
}

func (tb *TelegramBot) handleStatus(ctx context.Context, u chatmodel.TelegramUpdate) error {
	conv, err := tb.Broker.GetActiveConversation(ctx, u.ChatID, u.ThreadID)
	if err != nil {
		return err
	}
	if conv == nil {
		return tb.replyText(ctx, u, "no active conversation")
	}
	msg := fmt.Sprintf(
		"active conversation\ncontainer: %s\nworkspace: %s\ninitialized: %t",
		conv.ContainerName,
		conv.WorkspaceHost,
		conv.Initialized,
	)
	if strings.TrimSpace(conv.LastError) != "" {
		msg += "\nlast_error: " + conv.LastError
	}
	return tb.replyText(ctx, u, msg)
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
