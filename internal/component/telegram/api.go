package telegram

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

	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type TelegramAPI interface {
	Run(ctx context.Context, pollTimeout time.Duration, onUpdate func(ctx context.Context, u TelegramUpdate)) error
	SendMessage(ctx context.Context, chatID int64, threadID int, text string, parseMode string) error
	SendDocument(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error
	SendPhoto(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error
	SendVideo(ctx context.Context, chatID int64, threadID int, caption string, media message.Media) error
	SendAudio(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error
	SendChatAction(ctx context.Context, chatID int64, threadID int, action message.ChatAction) error
	DownloadFile(ctx context.Context, fileID string) ([]byte, error)
}

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

func (a *TelegramAPIV2) Run(ctx context.Context, pollTimeout time.Duration, onUpdate func(ctx context.Context, u TelegramUpdate)) error {
	if onUpdate == nil {
		return fmt.Errorf("onUpdate is nil")
	}

	opts := []bot.Option{
		bot.WithDefaultHandler(func(hctx context.Context, b *bot.Bot, upd *models.Update) {
			msg := telegramUpdateMessage(upd)
			if msg == nil {
				return
			}
			tupd := TelegramUpdate{
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

	a.setBot(b)

	b.Start(ctx)
	return ctx.Err()
}

func telegramUpdateMessage(upd *models.Update) *models.Message {
	if upd == nil {
		return nil
	}
	switch {
	case upd.Message != nil:
		return upd.Message
	case upd.EditedMessage != nil:
		return upd.EditedMessage
	case upd.ChannelPost != nil:
		return upd.ChannelPost
	case upd.EditedChannelPost != nil:
		return upd.EditedChannelPost
	case upd.BusinessMessage != nil:
		return upd.BusinessMessage
	case upd.EditedBusinessMessage != nil:
		return upd.EditedBusinessMessage
	default:
		return nil
	}
}

func (a *TelegramAPIV2) SendMessage(ctx context.Context, chatID int64, threadID int, text string, parseMode string) error {
	b, err := a.ensureBot()
	if err != nil {
		return err
	}

	p := &bot.SendMessageParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		Text:            text,
	}
	if strings.TrimSpace(parseMode) != "" {
		p.ParseMode = models.ParseMode(parseMode)
	}

	_, err = b.SendMessage(ctx, p)
	return err
}

func (a *TelegramAPIV2) SendDocument(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error {
	b, err := a.ensureBot()
	if err != nil {
		return err
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
	_, err = b.SendDocument(ctx, p)
	return err
}

func (a *TelegramAPIV2) SendPhoto(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error {
	b, err := a.ensureBot()
	if err != nil {
		return err
	}
	p := &bot.SendPhotoParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		Photo:           &models.InputFileUpload{Filename: strings.TrimSpace(filename), Data: bytes.NewReader(content)},
		Caption:         strings.TrimSpace(caption),
	}
	_, err = b.SendPhoto(ctx, p)
	return err
}

func (a *TelegramAPIV2) SendVideo(ctx context.Context, chatID int64, threadID int, caption string, media message.Media) error {
	b, err := a.ensureBot()
	if err != nil {
		return err
	}
	p := &bot.SendVideoParams{
		ChatID:            chatID,
		MessageThreadID:   threadID,
		Video:             &models.InputFileUpload{Filename: strings.TrimSpace(media.Filename), Data: bytes.NewReader(media.Content)},
		Caption:           strings.TrimSpace(caption),
		Width:             media.Width,
		Height:            media.Height,
		Duration:          media.DurationSeconds,
		SupportsStreaming: media.SupportsStreaming,
	}
	if media.Thumbnail != nil && len(media.Thumbnail.Content) > 0 {
		p.Thumbnail = &models.InputFileUpload{
			Filename: strings.TrimSpace(media.Thumbnail.Filename),
			Data:     bytes.NewReader(media.Thumbnail.Content),
		}
	}
	_, err = b.SendVideo(ctx, p)
	return err
}

func (a *TelegramAPIV2) SendAudio(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error {
	b, err := a.ensureBot()
	if err != nil {
		return err
	}
	p := &bot.SendAudioParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		Audio:           &models.InputFileUpload{Filename: strings.TrimSpace(filename), Data: bytes.NewReader(content)},
		Caption:         strings.TrimSpace(caption),
	}
	_, err = b.SendAudio(ctx, p)
	return err
}

func (a *TelegramAPIV2) SendChatAction(ctx context.Context, chatID int64, threadID int, action message.ChatAction) error {
	b, err := a.ensureBot()
	if err != nil {
		return err
	}
	p := &bot.SendChatActionParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		Action:          models.ChatAction(action),
	}
	_, err = b.SendChatAction(ctx, p)
	return err
}

func (a *TelegramAPIV2) DownloadFile(ctx context.Context, fileID string) ([]byte, error) {
	b, err := a.ensureBot()
	if err != nil {
		return nil, err
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

func (a *TelegramAPIV2) setBot(b *bot.Bot) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.b = b
}

func (a *TelegramAPIV2) ensureBot() (*bot.Bot, error) {
	if b := a.getBot(); b != nil {
		return b, nil
	}
	b, err := bot.New(a.token)
	if err != nil {
		return nil, err
	}
	a.setBot(b)
	return b, nil
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

func telegramMessageAttachments(msg *models.Message) []TelegramAttachment {
	if msg == nil {
		return nil
	}
	var out []TelegramAttachment

	if doc := msg.Document; doc != nil {
		out = append(out, TelegramAttachment{
			Kind:     "document",
			FileID:   doc.FileID,
			Filename: attachmentFilename("document", msg.ID, doc.FileName, doc.MimeType, ".bin"),
		})
	}
	if len(msg.Photo) > 0 {
		photo := msg.Photo[len(msg.Photo)-1]
		out = append(out, TelegramAttachment{
			Kind:     "photo",
			FileID:   photo.FileID,
			Filename: attachmentFilename("photo", msg.ID, "", "image/jpeg", ".jpg"),
		})
	}
	if video := msg.Video; video != nil {
		out = append(out, TelegramAttachment{
			Kind:     "video",
			FileID:   video.FileID,
			Filename: attachmentFilename("video", msg.ID, video.FileName, video.MimeType, ".mp4"),
		})
	}
	if audio := msg.Audio; audio != nil {
		out = append(out, TelegramAttachment{
			Kind:     "audio",
			FileID:   audio.FileID,
			Filename: attachmentFilename("audio", msg.ID, audio.FileName, audio.MimeType, ".bin"),
		})
	}
	if voice := msg.Voice; voice != nil {
		out = append(out, TelegramAttachment{
			Kind:     "voice",
			FileID:   voice.FileID,
			Filename: attachmentFilename("voice", msg.ID, "", voice.MimeType, ".ogg"),
		})
	}
	if animation := msg.Animation; animation != nil {
		out = append(out, TelegramAttachment{
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
