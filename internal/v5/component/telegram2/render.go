package telegram2

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	markdown "github.com/bartdeboer/ctgbot/internal/markdown"
)

const (
	telegramSemanticChunkSize = 3500
	telegramMessageMax        = 4096
)

func (c *Component) sendRenderedText(ctx context.Context, chatID int64, threadID int, text string) error {
	if c == nil || c.api == nil {
		return fmt.Errorf("missing telegram api")
	}
	text = cleanTextForTelegram(text)
	if text == "" {
		text = "(empty response)"
	}
	doc, err := markdown.Parse(text)
	if err != nil {
		c.logf("telegram markdown parse failed, falling back to plain split: %v", err)
		for _, chunk := range splitTelegramText(text, telegramSemanticChunkSize) {
			if err := c.api.SendMessage(ctx, chatID, threadID, chunk, ""); err != nil {
				return err
			}
		}
		return nil
	}
	chunkDocs := doc.Chunked(telegramSemanticChunkSize)
	for i, chunkDoc := range chunkDocs {
		if err := c.sendDocumentChunk(ctx, chatID, threadID, chunkDoc, i+1, len(chunkDocs)); err != nil {
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

func (c *Component) sendPlainChunk(ctx context.Context, chatID int64, threadID int, doc *markdown.Document, chunkIndex int, chunkCount int) error {
	for i, plainDoc := range doc.Chunked(telegramSemanticChunkSize) {
		text, err := plainDoc.Render(markdown.RenderOptions{Format: markdown.RenderPlain})
		if err != nil {
			return err
		}
		c.logf("telegram chunk %d/%d using plain fallback segment %d preview=%q", chunkIndex, chunkCount, i+1, telegramPreview(text))
		if err := c.api.SendMessage(ctx, chatID, threadID, text, ""); err != nil {
			return err
		}
	}
	return nil
}

func (c *Component) sendDocumentChunk(ctx context.Context, chatID int64, threadID int, doc *markdown.Document, chunkIndex int, chunkCount int) error {
	attempts := c.telegramRenderAttempts()
	for i, attempt := range attempts {
		text, err := doc.Render(markdown.RenderOptions{Format: attempt.format})
		if err != nil {
			c.logf("telegram chunk %d/%d %s render failed, trying fallback: %v", chunkIndex, chunkCount, attempt.name, err)
			continue
		}
		if telegramTextLen(text) > telegramMessageMax {
			c.logf("telegram chunk %d/%d %s exceeds telegram max len=%d preview=%q, falling back to plain", chunkIndex, chunkCount, attempt.name, telegramTextLen(text), telegramPreview(text))
			return c.sendPlainChunk(ctx, chatID, threadID, doc, chunkIndex, chunkCount)
		}
		if err := c.api.SendMessage(ctx, chatID, threadID, text, attempt.parseMode); err != nil {
			if attempt.parseMode != "" && isTelegramFormattingError(err) && i < len(attempts)-1 {
				c.logf("telegram chunk %d/%d %s send failed, trying fallback: %v preview=%q", chunkIndex, chunkCount, attempt.name, err, telegramPreview(text))
				continue
			}
			return err
		}
		if i > 0 {
			c.logf("telegram chunk %d/%d sent with fallback format=%s preview=%q", chunkIndex, chunkCount, attempt.name, telegramPreview(text))
		}
		return nil
	}
	return fmt.Errorf("no telegram render mode succeeded")
}

func (c *Component) telegramRenderAttempts() []telegramRenderAttempt {
	preferred := telegramRenderAttempt{format: markdown.RenderPlain, parseMode: "", name: "plain"}
	if c != nil && c.cfg != nil {
		switch c.cfg.Telegram().RenderFormat() {
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

func telegramPreview(text string) string {
	text = strings.ReplaceAll(strings.TrimSpace(text), "\n", " ")
	if telegramTextLen(text) <= 80 {
		return text
	}
	r := []rune(text)
	return string(r[:80]) + "…"
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
