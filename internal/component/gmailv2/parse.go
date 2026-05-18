package gmailv2

import (
	"encoding/base64"
	"html"
	"strings"
	"unicode"
	"unicode/utf8"

	gmailapi "google.golang.org/api/gmail/v1"
)

const (
	maxRenderedEmailBodyRunes = 20000
	maxRenderedHeaderRunes    = 500
	maxRenderedSubjectRunes   = 255
)

var selectedPromptMetadataHeaders = []string{
	"Message-ID",
	"Reply-To",
	"In-Reply-To",
	"References",
	"List-ID",
	"List-Unsubscribe",
	"List-Unsubscribe-Post",
	"Auto-Submitted",
	"Precedence",
	"Feedback-ID",
}

func headerValue(message *gmailapi.Message, name string) string {
	if message == nil || message.Payload == nil {
		return ""
	}
	for _, header := range message.Payload.Headers {
		if header == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(header.Name), strings.TrimSpace(name)) {
			return strings.TrimSpace(header.Value)
		}
	}
	return ""
}

func renderedHeaderValue(value string) string {
	return renderedInlineValue(value, maxRenderedHeaderRunes)
}

func renderedSubjectValue(value string) string {
	return renderedInlineValue(value, maxRenderedSubjectRunes)
}

func renderedInlineValue(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t':
			return ' '
		}
		if r < 0x20 || r == 0x7f || isPromptFormattingRune(r) {
			return -1
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	return capRunesInline(value, maxRunes)
}

func sanitizePromptBody(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		switch {
		case r == '`':
			continue
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r == '\r':
			continue
		case r < 0x20 || r == 0x7f:
			continue
		case isPromptFormattingRune(r):
			continue
		default:
			b.WriteRune(r)
		}
	}
	return capRunes(b.String(), maxRenderedEmailBodyRunes)
}

func isPromptFormattingRune(r rune) bool {
	switch r {
	case '\u200b', '\u200c', '\u200d', '\u200e', '\u200f', '\u202a', '\u202b', '\u202c', '\u202d', '\u202e', '\u2060', '\ufeff':
		return true
	}
	return r >= '\u2066' && r <= '\u2069'
}

func capRunes(value string, max int) string {
	if max <= 0 || utf8.RuneCountInString(value) <= max {
		return value
	}
	var b strings.Builder
	count := 0
	for _, r := range value {
		if count >= max {
			break
		}
		b.WriteRune(r)
		count++
	}
	return strings.TrimRight(b.String(), " \t\n") + "\n[truncated]"
}

func capRunesInline(value string, max int) string {
	if max <= 0 || utf8.RuneCountInString(value) <= max {
		return value
	}
	var b strings.Builder
	count := 0
	for _, r := range value {
		if count >= max {
			break
		}
		b.WriteRune(r)
		count++
	}
	return strings.TrimSpace(b.String()) + " [truncated]"
}

func collectBodyText(part *gmailapi.MessagePart) (plain string, htmlBody string) {
	if part == nil {
		return "", ""
	}
	if strings.TrimSpace(part.Filename) == "" && part.Body != nil && strings.TrimSpace(part.Body.Data) != "" {
		text := decodeGmailBody(part.Body.Data)
		switch strings.ToLower(strings.TrimSpace(part.MimeType)) {
		case "text/plain":
			plain = appendParagraph(plain, text)
		case "text/html":
			htmlBody = appendParagraph(htmlBody, text)
		}
	}
	for _, child := range part.Parts {
		childPlain, childHTML := collectBodyText(child)
		plain = appendParagraph(plain, childPlain)
		htmlBody = appendParagraph(htmlBody, childHTML)
	}
	return strings.TrimSpace(plain), strings.TrimSpace(htmlBody)
}

func decodeGmailBody(data string) string {
	decoded := decodeGmailBytes(data)
	if len(decoded) == 0 {
		return ""
	}
	return strings.TrimSpace(string(decoded))
}

func decodeGmailBytes(data string) []byte {
	data = strings.TrimSpace(data)
	if data == "" {
		return nil
	}
	decoded, err := base64.URLEncoding.DecodeString(data)
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(data)
	}
	if err != nil {
		return nil
	}
	return decoded
}

func stripHTML(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	inTag := false
	lastSpace := false
	for _, r := range value {
		switch {
		case r == '<':
			inTag = true
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
			}
		case r == '>':
			inTag = false
		case inTag:
			continue
		case unicode.IsSpace(r):
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
			}
		default:
			b.WriteRune(r)
			lastSpace = false
		}
	}
	return strings.TrimSpace(html.UnescapeString(b.String()))
}

func appendParagraph(base string, extra string) string {
	base = strings.TrimSpace(base)
	extra = strings.TrimSpace(extra)
	if extra == "" {
		return base
	}
	if base == "" {
		return extra
	}
	return base + "\n\n" + extra
}
