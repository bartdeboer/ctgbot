package botengine

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/bartdeboer/go-clir"
)

type tgUpdateKey struct{}

type TelegramBot struct {
	API      TelegramAPI
	Storage  *ConversationStorage
	Sessions *SessionExecutor
	Config   *Config
	Logger   *log.Logger
	router   *clir.Router
}

func NewTelegramBot(api TelegramAPI, storage *ConversationStorage, sessions *SessionExecutor, cfg *Config, logger *log.Logger) *TelegramBot {
	return &TelegramBot{
		API:      api,
		Storage:  storage,
		Sessions: sessions,
		Config:   cfg,
		Logger:   logger,
	}
}

func (tb *TelegramBot) AutoMigrate(ctx context.Context) error {
	return tb.Storage.AutoMigrate(ctx)
}

func (tb *TelegramBot) Run(ctx context.Context) error {
	if tb.Config == nil {
		return fmt.Errorf("missing config")
	}
	if err := tb.Config.EnsurePaths(); err != nil {
		return err
	}

	tb.router = tb.buildRouter()
	return tb.API.Run(ctx, tb.Config.PollTimeout(), func(cbCtx context.Context, u TelegramUpdate) {
		tb.handleUpdate(cbCtx, u)
	})
}

func (tb *TelegramBot) buildRouter() *clir.Router {
	r := clir.New()

	r.Routes(func(b *clir.Builder) {
		tg := clir.WithContext(b, func(req *clir.Request) (TelegramUpdate, error) {
			v := req.Context().Value(tgUpdateKey{})
			u, ok := v.(TelegramUpdate)
			if !ok {
				return TelegramUpdate{}, fmt.Errorf("missing TelegramUpdate in context")
			}
			return u, nil
		})

		tg.Handle("new", "Start a new Codex conversation", func(req *clir.Request, u TelegramUpdate) error {
			workspace := ""
			if len(req.Extra) > 0 {
				workspace = req.Extra[0]
			}
			return tb.handleNewConversation(req.Context(), u, workspace)
		})

		tg.Handle("stop", "Stop the active Codex conversation", func(req *clir.Request, u TelegramUpdate) error {
			return tb.handleStopConversation(req.Context(), u)
		})

		tg.Handle("status", "Show the active Codex conversation", func(req *clir.Request, u TelegramUpdate) error {
			return tb.handleStatus(req.Context(), u)
		})

		tg.Handle("help", "Show available commands", func(req *clir.Request, u TelegramUpdate) error {
			return tb.replyText(req.Context(), u, "Commands:\n/new [absolute-host-path]\n/status\n/stop\n/help\n\nAny non-command message is sent to the active Codex conversation.")
		})
	})

	return r
}

func (tb *TelegramBot) handleUpdate(ctx context.Context, u TelegramUpdate) {
	text := strings.TrimSpace(u.Text)
	if text == "" {
		return
	}

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
		return
	}

	if strings.HasPrefix(text, "/") {
		args := normalizeTelegramCommand(text)
		if len(args) == 0 {
			return
		}
		cmdCtx := context.WithValue(ctx, tgUpdateKey{}, u)
		if err := tb.router.Run(cmdCtx, args); err != nil {
			_ = tb.replyText(ctx, u, fmt.Sprintf("command error: %v", err))
		}
		return
	}

	if err := tb.handlePrompt(ctx, u, text); err != nil {
		_ = tb.replyText(ctx, u, fmt.Sprintf("conversation error: %v", err))
	}
}

func (tb *TelegramBot) handleNewConversation(ctx context.Context, u TelegramUpdate, workspace string) error {
	current, err := tb.Storage.GetActive(ctx, u.ChatID, u.ThreadID)
	if err != nil {
		return err
	}
	if current != nil {
		_ = tb.Sessions.StopConversation(ctx, current)
		_ = tb.Storage.MarkStopped(ctx, current.ID, "replaced by /new")
	}

	conv, err := tb.Sessions.StartConversation(ctx, u.ChatID, u.ThreadID, workspace)
	if err != nil {
		return err
	}
	if err := tb.Storage.Create(ctx, conv); err != nil {
		_ = tb.Sessions.StopConversation(ctx, conv)
		return err
	}

	msg := fmt.Sprintf("conversation started\ncontainer: %s\nworkspace: %s", conv.ContainerName, conv.WorkspaceHost)
	return tb.replyText(ctx, u, msg)
}

func (tb *TelegramBot) handleStopConversation(ctx context.Context, u TelegramUpdate) error {
	conv, err := tb.Storage.GetActive(ctx, u.ChatID, u.ThreadID)
	if err != nil {
		return err
	}
	if conv == nil {
		return tb.replyText(ctx, u, "no active conversation")
	}
	if err := tb.Sessions.StopConversation(ctx, conv); err != nil {
		return err
	}
	if err := tb.Storage.MarkStopped(ctx, conv.ID, "stopped by /stop"); err != nil {
		return err
	}
	return tb.replyText(ctx, u, "conversation stopped")
}

func (tb *TelegramBot) handleStatus(ctx context.Context, u TelegramUpdate) error {
	conv, err := tb.Storage.GetActive(ctx, u.ChatID, u.ThreadID)
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

func (tb *TelegramBot) handlePrompt(ctx context.Context, u TelegramUpdate, prompt string) error {
	conv, err := tb.Storage.GetActive(ctx, u.ChatID, u.ThreadID)
	if err != nil {
		return err
	}
	if conv == nil {
		return tb.replyText(ctx, u, "no active conversation. Start one with /new")
	}

	reply, runErr := tb.Sessions.SendPrompt(ctx, conv, prompt)
	if reply != "" {
		reply = cleanTextForTelegram(reply)
	}

	if conv.ID != 0 && !conv.Initialized && runErr == nil {
		conv.Initialized = true
		_ = tb.Storage.MarkInitialized(ctx, conv.ID)
	}
	if conv.ID != 0 {
		lastErr := ""
		if runErr != nil {
			lastErr = runErr.Error()
		}
		_ = tb.Storage.MarkError(ctx, conv.ID, lastErr)
	}

	if reply != "" {
		if err := tb.replyText(ctx, u, reply); err != nil {
			return err
		}
	}
	return runErr
}

func (tb *TelegramBot) replyText(ctx context.Context, u TelegramUpdate, text string) error {
	text = cleanTextForTelegram(text)
	if text == "" {
		text = "(empty response)"
	}

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
