package telegram

import (
	"context"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/message"
)

func (c *Component) loadIncomingAttachments(ctx context.Context, attachments []TelegramAttachment) ([]message.Media, error) {
	out := make([]message.Media, 0, len(attachments))
	for _, attachment := range attachments {
		content, err := c.api.DownloadFile(ctx, attachment.FileID)
		if err != nil {
			return nil, err
		}
		out = append(out, message.Media{
			Kind:     strings.TrimSpace(attachment.Kind),
			Filename: strings.TrimSpace(attachment.Filename),
			Content:  content,
		})
	}
	return out, nil
}

func (c *Component) sendAttachment(ctx context.Context, chatID int64, threadID int, caption string, media message.Media) error {
	contentType := strings.TrimSpace(strings.ToLower(media.ContentType))
	switch {
	case contentType == "text/markdown":
		text := string(media.Content)
		if strings.TrimSpace(caption) != "" {
			text = strings.TrimSpace(caption) + "\n\n" + text
		}
		return c.sendRenderedText(ctx, chatID, threadID, text)
	case isTelegramTextualAttachment(contentType) && strings.TrimSpace(media.Syntax) != "":
		text, ok := renderTelegramTextAttachment(caption, media)
		if !ok {
			return c.api.SendDocument(ctx, chatID, threadID, media.Filename, caption, media.Content)
		}
		return c.sendRenderedText(ctx, chatID, threadID, text)
	case isTelegramTextualAttachment(contentType):
		text := string(media.Content)
		if strings.TrimSpace(caption) != "" {
			text = strings.TrimSpace(caption) + "\n\n" + text
		}
		return c.api.SendMessage(ctx, chatID, threadID, text, "")
	case strings.HasPrefix(contentType, "image/"):
		return c.api.SendPhoto(ctx, chatID, threadID, media.Filename, caption, media.Content)
	case strings.HasPrefix(contentType, "video/"):
		return c.api.SendVideo(ctx, chatID, threadID, caption, media)
	case strings.EqualFold(strings.TrimSpace(media.Kind), "voice"):
		return c.api.SendVoice(ctx, chatID, threadID, caption, media)
	case strings.HasPrefix(contentType, "audio/"):
		return c.api.SendAudio(ctx, chatID, threadID, media.Filename, caption, media.Content)
	default:
		return c.api.SendDocument(ctx, chatID, threadID, media.Filename, caption, media.Content)
	}
}

func isTelegramTextualAttachment(contentType string) bool {
	contentType = strings.TrimSpace(strings.ToLower(contentType))
	if strings.HasPrefix(contentType, "text/") {
		return true
	}
	switch contentType {
	case "application/json", "application/xml", "application/yaml", "application/x-yaml":
		return true
	default:
		return false
	}
}

func renderTelegramTextAttachment(caption string, media message.Media) (string, bool) {
	body := string(media.Content)
	var b strings.Builder
	if strings.TrimSpace(caption) != "" {
		b.WriteString(strings.TrimSpace(caption))
		b.WriteString("\n\n")
	}
	b.WriteString("```")
	b.WriteString(strings.TrimSpace(media.Syntax))
	b.WriteString("\n")
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("```")
	return b.String(), true
}
