package gmail

import (
	"encoding/base64"
	"html"
	"strings"
	"unicode"

	gmailapi "google.golang.org/api/gmail/v1"
)

func emailPromptText(message *gmailapi.Message) string {
	if message == nil {
		return ""
	}
	subject := headerValue(message, "Subject")
	from := headerValue(message, "From")
	date := headerValue(message, "Date")
	body := messageBodyText(message)
	if body == "" {
		body = strings.TrimSpace(message.Snippet)
	}
	var lines []string
	if subject != "" {
		lines = append(lines, "Subject: "+subject)
	}
	if from != "" {
		lines = append(lines, "From: "+from)
	}
	if date != "" {
		lines = append(lines, "Date: "+date)
	}
	if body != "" {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, body)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
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

func messageBodyText(message *gmailapi.Message) string {
	if message == nil || message.Payload == nil {
		return ""
	}
	plain, htmlBody := collectBodyText(message.Payload)
	if plain != "" {
		return plain
	}
	return stripHTML(htmlBody)
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
	data = strings.TrimSpace(data)
	if data == "" {
		return ""
	}
	decoded, err := base64.URLEncoding.DecodeString(data)
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(data)
	}
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(decoded))
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
