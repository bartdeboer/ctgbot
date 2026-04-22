package telegramengine

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/chatmodel"
	markdown "github.com/bartdeboer/ctgbot/internal/markdown"
	"github.com/bartdeboer/ctgbot/internal/messenger"
)

type tgEventKey struct{}

type TelegramBot struct {
	API     TelegramAPI
	Updates *UpdateStorage
	Config  *appstate.Config
	Logger  *log.Logger
}

var chatActionRefreshInterval = 4 * time.Second

const (
	telegramSemanticChunkSize = 3500
	telegramMessageMax        = 4096
)

func NewTelegramBot(api TelegramAPI, updates *UpdateStorage, cfg *appstate.Config, logger *log.Logger) *TelegramBot {
	return &TelegramBot{
		API:     api,
		Updates: updates,
		Config:  cfg,
		Logger:  logger,
	}
}

func (tb *TelegramBot) ProviderType() string {
	return "telegram"
}

func (tb *TelegramBot) SendAgentResponse(ctx context.Context, msg messenger.ResolvedOutgoingMessage) error {
	chatID, err := strconv.ParseInt(strings.TrimSpace(msg.ProviderChatID), 10, 64)
	if err != nil {
		return fmt.Errorf("parse telegram chat id: %w", err)
	}
	threadID, err := parseTelegramProviderThreadID(msg.ProviderThreadID)
	if err != nil {
		return err
	}
	contentType := strings.TrimSpace(strings.ToLower(msg.ContentType))
	switch contentType {
	case "", "text/markdown":
		return tb.sendRenderedText(ctx, chatID, threadID, 0, msg.Text)
	case "text/plain":
		return tb.API.SendMessage(ctx, chatID, threadID, 0, msg.Text, "")
	default:
		return tb.sendRenderedText(ctx, chatID, threadID, 0, msg.Text)
	}
}

func (tb *TelegramBot) SendMedia(ctx context.Context, file messenger.ResolvedOutgoingFile) error {
	chatID, err := strconv.ParseInt(strings.TrimSpace(file.ProviderChatID), 10, 64)
	if err != nil {
		return fmt.Errorf("parse telegram chat id: %w", err)
	}
	threadID, err := parseTelegramProviderThreadID(file.ProviderThreadID)
	if err != nil {
		return err
	}
	contentType := strings.TrimSpace(strings.ToLower(file.ContentType))
	switch {
	case contentType == "text/markdown":
		text := string(file.Content)
		if strings.TrimSpace(file.Caption) != "" {
			text = strings.TrimSpace(file.Caption) + "\n\n" + text
		}
		return tb.sendRenderedText(ctx, chatID, threadID, 0, text)
	case contentType == "text/plain" && strings.TrimSpace(file.Syntax) != "":
		text, ok := renderTelegramTextAttachment(file)
		if !ok {
			return tb.API.SendDocument(ctx, chatID, threadID, file.Filename, file.Caption, file.Content)
		}
		return tb.sendRenderedText(ctx, chatID, threadID, 0, text)
	case contentType == "text/plain":
		text := string(file.Content)
		if strings.TrimSpace(file.Caption) != "" {
			text = strings.TrimSpace(file.Caption) + "\n\n" + text
		}
		return tb.API.SendMessage(ctx, chatID, threadID, 0, text, "")
	case strings.HasPrefix(contentType, "image/"):
		return tb.API.SendPhoto(ctx, chatID, threadID, file.Filename, file.Caption, file.Content)
	case strings.HasPrefix(contentType, "video/"):
		return tb.API.SendVideo(ctx, chatID, threadID, file.Filename, file.Caption, file.Content)
	case strings.HasPrefix(contentType, "audio/"):
		return tb.API.SendAudio(ctx, chatID, threadID, file.Filename, file.Caption, file.Content)
	default:
		return tb.API.SendDocument(ctx, chatID, threadID, file.Filename, file.Caption, file.Content)
	}
}

func renderTelegramTextAttachment(file messenger.ResolvedOutgoingFile) (string, bool) {
	body := string(file.Content)
	if strings.Contains(body, "```") {
		return "", false
	}
	var b strings.Builder
	if strings.TrimSpace(file.Caption) != "" {
		b.WriteString(strings.TrimSpace(file.Caption))
		b.WriteString("\n\n")
	}
	b.WriteString("```")
	b.WriteString(strings.TrimSpace(file.Syntax))
	b.WriteString("\n")
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("```")
	return b.String(), true
}

func (tb *TelegramBot) StartChatAction(ctx context.Context, target messenger.ChatTarget, action messenger.ChatAction) (func(), error) {
	chatID, err := strconv.ParseInt(strings.TrimSpace(target.ProviderChatID), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse telegram chat id: %w", err)
	}
	threadID, err := parseTelegramProviderThreadID(target.ProviderThreadID)
	if err != nil {
		return nil, err
	}
	if err := tb.API.SendChatAction(ctx, chatID, threadID, action); err != nil {
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
				if err := tb.API.SendChatAction(runCtx, chatID, threadID, action); err != nil {
					tb.logf("telegram chat action failed chat=%d thread=%d action=%q err=%v", chatID, threadID, action, err)
					return
				}
			}
		}
	}()

	return stop, nil
}

func (tb *TelegramBot) AutoMigrate(ctx context.Context) error {
	if tb.Updates != nil {
		if err := tb.Updates.AutoMigrate(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (tb *TelegramBot) Run(ctx context.Context, onUpdate func(context.Context, messenger.IncomingUpdate) (messenger.IncomingResult, error)) error {
	if tb.Config == nil {
		return fmt.Errorf("missing config")
	}
	if err := tb.Config.EnsurePaths(); err != nil {
		return err
	}
	handler := func(cbCtx context.Context, u chatmodel.TelegramUpdate) {
		tb.handleUpdate(cbCtx, u, onUpdate)
	}
	if window := tb.Config.TelegramDebounceWindow(); window > 0 {
		return NewDebouncer(window, tb.Logger, handler).Run(ctx, tb.API, tb.Config.TelegramPollTimeout())
	}
	return tb.API.Run(ctx, tb.Config.TelegramPollTimeout(), handler)
}

func (tb *TelegramBot) handleUpdate(ctx context.Context, u chatmodel.TelegramUpdate, onUpdate func(context.Context, messenger.IncomingUpdate) (messenger.IncomingResult, error)) {
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
	if err := tb.handleUpdateSerialized(ctx, u, text, onUpdate); err != nil {
		tb.recordEventError(ctx, err)
		tb.logf("telegram update handling failed chat=%d thread=%d msg=%d err=%v", u.ChatID, u.ThreadID, u.MessageID, err)
	}
}

func (tb *TelegramBot) handleUpdateSerialized(ctx context.Context, u chatmodel.TelegramUpdate, text string, onUpdate func(context.Context, messenger.IncomingUpdate) (messenger.IncomingResult, error)) error {
	if onUpdate == nil {
		return fmt.Errorf("missing update callback")
	}
	update := messenger.IncomingUpdate{
		ProviderType:      "telegram",
		ProviderChatID:    fmt.Sprintf("%d", u.ChatID),
		ProviderThreadID:  fmt.Sprintf("%d", u.ThreadID),
		Text:              text,
		ChatLabel:         strings.TrimSpace(u.ChatTitle),
		UserLabel:         u.UserLabel(),
		UserID:            u.UserID,
		IsAdmin:           tb.Config != nil && u.UserID != 0 && u.UserID == tb.Config.TelegramAdminUserID(),
		ProviderMessageID: fmt.Sprintf("%d", u.MessageID),
	}

	tb.logf("telegram update chat=%d thread=%d msg=%d user=%q text=%q attachments=%d", u.ChatID, u.ThreadID, u.MessageID, u.UserLabel(), text, len(u.Attachments))

	if len(u.Attachments) > 0 {
		attachments, err := tb.loadIncomingAttachments(ctx, u.Attachments)
		if err != nil {
			return fmt.Errorf("load incoming attachments: %w", err)
		}
		update.Attachments = attachments
	}

	result, err := onUpdate(ctx, update)
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

func (tb *TelegramBot) loadIncomingAttachments(ctx context.Context, attachments []chatmodel.TelegramAttachment) ([]messenger.IncomingAttachment, error) {
	out := make([]messenger.IncomingAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		content, err := tb.API.DownloadFile(ctx, attachment.FileID)
		if err != nil {
			return nil, err
		}
		out = append(out, messenger.IncomingAttachment{
			Kind:     strings.TrimSpace(attachment.Kind),
			Filename: strings.TrimSpace(attachment.Filename),
			Content:  content,
		})
	}
	return out, nil
}

func (tb *TelegramBot) replyText(ctx context.Context, u chatmodel.TelegramUpdate, text string) error {
	text = cleanTextForTelegram(text)
	if text == "" {
		text = "(empty response)"
	}
	tb.appendEventResponse(ctx, text)
	return tb.sendRenderedText(ctx, u.ChatID, u.ThreadID, u.MessageID, text)
}

func (tb *TelegramBot) sendRenderedText(ctx context.Context, chatID int64, threadID int, replyTo int, text string) error {
	text = cleanTextForTelegram(text)
	if text == "" {
		text = "(empty response)"
	}
	doc, err := markdown.Parse(text)
	if err != nil {
		tb.logf("telegram markdown parse failed, falling back to plain split: %v", err)
		for _, chunk := range splitTelegramText(text, telegramSemanticChunkSize) {
			if err := tb.API.SendMessage(ctx, chatID, threadID, replyTo, chunk, ""); err != nil {
				return err
			}
		}
		return nil
	}
	chunkDocs := doc.Chunked(telegramSemanticChunkSize)
	for i, chunkDoc := range chunkDocs {
		if err := tb.sendDocumentChunk(ctx, chatID, threadID, replyTo, chunkDoc, i+1, len(chunkDocs)); err != nil {
			return err
		}
	}
	return nil
}

type telegramRenderAttempt struct {
	format    markdown.RenderFormat
	parseMode string
	name      string
}

func (tb *TelegramBot) sendPlainChunk(ctx context.Context, chatID int64, threadID int, replyTo int, doc *markdown.Document, chunkIndex int, chunkCount int) error {
	for i, plainDoc := range doc.Chunked(telegramSemanticChunkSize) {
		text, err := plainDoc.Render(markdown.RenderOptions{Format: markdown.RenderPlain})
		if err != nil {
			return err
		}
		tb.logf("telegram chunk %d/%d using plain fallback segment %d preview=%q", chunkIndex, chunkCount, i+1, telegramPreview(text))
		if err := tb.API.SendMessage(ctx, chatID, threadID, replyTo, text, ""); err != nil {
			return err
		}
	}
	return nil
}

func (tb *TelegramBot) sendDocumentChunk(ctx context.Context, chatID int64, threadID int, replyTo int, doc *markdown.Document, chunkIndex int, chunkCount int) error {
	attempts := tb.telegramRenderAttempts()
	for i, attempt := range attempts {
		text, err := doc.Render(markdown.RenderOptions{Format: attempt.format})
		if err != nil {
			tb.logf("telegram chunk %d/%d %s render failed, trying fallback: %v", chunkIndex, chunkCount, attempt.name, err)
			continue
		}
		if telegramTextLen(text) > telegramMessageMax {
			tb.logf("telegram chunk %d/%d %s exceeds telegram max len=%d preview=%q, falling back to plain", chunkIndex, chunkCount, attempt.name, telegramTextLen(text), telegramPreview(text))
			return tb.sendPlainChunk(ctx, chatID, threadID, replyTo, doc, chunkIndex, chunkCount)
		}
		if err := tb.API.SendMessage(ctx, chatID, threadID, replyTo, text, attempt.parseMode); err != nil {
			if attempt.parseMode != "" && isTelegramFormattingError(err) && i < len(attempts)-1 {
				tb.logf("telegram chunk %d/%d %s send failed, trying fallback: %v preview=%q", chunkIndex, chunkCount, attempt.name, err, telegramPreview(text))
				continue
			}
			return err
		}
		if i > 0 {
			tb.logf("telegram chunk %d/%d sent with fallback format=%s preview=%q", chunkIndex, chunkCount, attempt.name, telegramPreview(text))
		}
		return nil
	}
	return fmt.Errorf("no telegram render mode succeeded")
}

func telegramPreview(text string) string {
	text = strings.ReplaceAll(strings.TrimSpace(text), "\n", " ")
	if telegramTextLen(text) <= 80 {
		return text
	}
	r := []rune(text)
	return string(r[:80]) + "…"
}
func (tb *TelegramBot) telegramRenderAttempts() []telegramRenderAttempt {
	preferred := telegramRenderAttempt{format: markdown.RenderPlain, parseMode: "", name: "plain"}
	if tb != nil && tb.Config != nil {
		switch tb.Config.TelegramRenderFormat() {
		case "html":
			preferred = telegramRenderAttempt{format: markdown.RenderHTML, parseMode: "HTML", name: "html"}
		case "markdown_v2":
			preferred = telegramRenderAttempt{format: markdown.RenderMarkdownV2, parseMode: "MarkdownV2", name: "markdown_v2"}
		}
	}
	all := []telegramRenderAttempt{
		preferred,
		{format: markdown.RenderHTML, parseMode: "HTML", name: "html"},
		{format: markdown.RenderPlain, parseMode: "", name: "plain"},
	}
	seen := map[string]bool{}
	out := make([]telegramRenderAttempt, 0, len(all))
	for _, attempt := range all {
		key := string(attempt.format) + "|" + attempt.parseMode
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, attempt)
	}
	return out
}

func isTelegramFormattingError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{"can't parse entities", "parse entities", "unsupported start tag", "entity"} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}
func telegramTextLen(text string) int {
	return utf8.RuneCountInString(text)
}

func (tb *TelegramBot) logf(format string, args ...any) {
	if tb.Logger != nil {
		b := tb.Logger
		b.Printf(format, args...)
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
