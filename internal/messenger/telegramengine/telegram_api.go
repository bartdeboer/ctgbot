package telegramengine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bartdeboer/ctgbot/internal/chatmodel"
	"github.com/bartdeboer/ctgbot/internal/messenger"
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
				ChatID:      msg.Chat.ID,
				ChatTitle:   msg.Chat.Title,
				ThreadID:    msg.MessageThreadID,
				MessageID:   msg.ID,
				Text:        telegramMessageText(msg),
				Attachments: telegramMessageAttachments(msg),
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

func (a *TelegramAPIV2) SendMessage(ctx context.Context, chatID int64, threadID int, replyTo int, text string, parseMode string) error {
	b := a.getBot()
	if b == nil {
		return fmt.Errorf("telegram bot not initialized")
	}

	p := &bot.SendMessageParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		Text:            text,
	}
	if strings.TrimSpace(parseMode) != "" {
		p.ParseMode = models.ParseMode(parseMode)
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
func (a *TelegramAPIV2) SendPhoto(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error {
	b := a.getBot()
	if b == nil {
		return fmt.Errorf("telegram bot not initialized")
	}
	p := &bot.SendPhotoParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		Photo:           &models.InputFileUpload{Filename: strings.TrimSpace(filename), Data: bytes.NewReader(content)},
		Caption:         strings.TrimSpace(caption),
	}
	_, err := b.SendPhoto(ctx, p)
	return err
}

func (a *TelegramAPIV2) SendVideo(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error {
	b := a.getBot()
	if b == nil {
		return fmt.Errorf("telegram bot not initialized")
	}
	p := &bot.SendVideoParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		Video:           &models.InputFileUpload{Filename: strings.TrimSpace(filename), Data: bytes.NewReader(content)},
		Caption:         strings.TrimSpace(caption),
	}
	_, err := b.SendVideo(ctx, p)
	return err
}

func (a *TelegramAPIV2) SendAudio(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error {
	b := a.getBot()
	if b == nil {
		return fmt.Errorf("telegram bot not initialized")
	}
	p := &bot.SendAudioParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		Audio:           &models.InputFileUpload{Filename: strings.TrimSpace(filename), Data: bytes.NewReader(content)},
		Caption:         strings.TrimSpace(caption),
	}
	_, err := b.SendAudio(ctx, p)
	return err
}

func (a *TelegramAPIV2) SendChatAction(ctx context.Context, chatID int64, threadID int, action messenger.ChatAction) error {
	b := a.getBot()
	if b == nil {
		return fmt.Errorf("telegram bot not initialized")
	}
	p := &bot.SendChatActionParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		Action:          models.ChatAction(action),
	}
	_, err := b.SendChatAction(ctx, p)
	return err
}

func (a *TelegramAPIV2) DownloadFile(ctx context.Context, fileID string) ([]byte, error) {
	b := a.getBot()
	if b == nil {
		return nil, fmt.Errorf("telegram bot not initialized")
	}
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return nil, fmt.Errorf("missing file id")
	}
	f, err := b.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return nil, err
	}
	if f == nil || strings.TrimSpace(f.FilePath) == "" {
		return nil, fmt.Errorf("telegram file path missing")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.FileDownloadLink(f), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download telegram file: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func (a *TelegramAPIV2) getBot() *bot.Bot {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.b
}

func telegramMessageText(msg *models.Message) string {
	if msg == nil {
		return ""
	}
	if strings.TrimSpace(msg.Text) != "" {
		return msg.Text
	}
	return strings.TrimSpace(msg.Caption)
}

func telegramMessageAttachments(msg *models.Message) []chatmodel.TelegramAttachment {
	if msg == nil {
		return nil
	}
	var out []chatmodel.TelegramAttachment

	if doc := msg.Document; doc != nil {
		out = append(out, chatmodel.TelegramAttachment{
			Kind:     "document",
			FileID:   doc.FileID,
			Filename: attachmentFilename("document", msg.ID, doc.FileName, doc.MimeType, ".bin"),
		})
	}
	if len(msg.Photo) > 0 {
		photo := msg.Photo[len(msg.Photo)-1]
		out = append(out, chatmodel.TelegramAttachment{
			Kind:     "photo",
			FileID:   photo.FileID,
			Filename: attachmentFilename("photo", msg.ID, "", "image/jpeg", ".jpg"),
		})
	}
	if video := msg.Video; video != nil {
		out = append(out, chatmodel.TelegramAttachment{
			Kind:     "video",
			FileID:   video.FileID,
			Filename: attachmentFilename("video", msg.ID, video.FileName, video.MimeType, ".mp4"),
		})
	}
	if audio := msg.Audio; audio != nil {
		out = append(out, chatmodel.TelegramAttachment{
			Kind:     "audio",
			FileID:   audio.FileID,
			Filename: attachmentFilename("audio", msg.ID, audio.FileName, audio.MimeType, ".bin"),
		})
	}
	if voice := msg.Voice; voice != nil {
		out = append(out, chatmodel.TelegramAttachment{
			Kind:     "voice",
			FileID:   voice.FileID,
			Filename: attachmentFilename("voice", msg.ID, "", voice.MimeType, ".ogg"),
		})
	}
	if animation := msg.Animation; animation != nil {
		out = append(out, chatmodel.TelegramAttachment{
			Kind:     "animation",
			FileID:   animation.FileID,
			Filename: attachmentFilename("animation", msg.ID, animation.FileName, animation.MimeType, ".bin"),
		})
	}

	return out
}

func attachmentFilename(kind string, messageID int, existing string, mimeType string, fallbackExt string) string {
	name := filepath.Base(strings.TrimSpace(existing))
	if name != "" && name != "." && name != string(filepath.Separator) {
		return name
	}
	ext := attachmentExtension(strings.TrimSpace(mimeType), fallbackExt)
	if ext == "" {
		ext = fallbackExt
	}
	return fmt.Sprintf("%s-%d%s", kind, messageID, ext)
}

func attachmentExtension(mimeType string, fallback string) string {
	if mimeType == "" {
		return fallback
	}
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "audio/mpeg":
		return ".mp3"
	case "audio/ogg", "audio/opus":
		return ".ogg"
	case "audio/mp4":
		return ".m4a"
	case "image/gif":
		return ".gif"
	}
	if exts, err := mime.ExtensionsByType(mimeType); err == nil && len(exts) > 0 {
		return exts[0]
	}
	return fallback
}
