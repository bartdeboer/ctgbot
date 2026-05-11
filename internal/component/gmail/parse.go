package gmail

import (
	"encoding/base64"
	"html"
	"net/mail"
	"strings"
	"unicode"

	gmailapi "google.golang.org/api/gmail/v1"
)

func emailPromptText(message *gmailapi.Message) string {
	return emailPromptTextForComponent("", message)
}

func emailPromptTextForComponent(componentRef string, message *gmailapi.Message) string {
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
	if metadata := gmailReplyMetadata(componentRef, message, from, subject); metadata != "" {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, metadata)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func gmailReplyMetadata(componentRef string, message *gmailapi.Message, from string, subject string) string {
	if message == nil {
		return ""
	}
	componentRef = strings.TrimSpace(componentRef)
	if componentRef == "" {
		componentRef = Type
	}
	messageID := strings.TrimSpace(message.Id)
	threadID := strings.TrimSpace(message.ThreadId)
	to := senderEmail(from)
	replySubject := replySubject(subject)
	inReplyTo := strings.TrimSpace(headerValue(message, "Message-ID"))

	var lines []string
	if messageID != "" {
		lines = append(lines, "Gmail message id: "+messageID)
	}
	if threadID != "" {
		lines = append(lines, "Gmail thread id: "+threadID)
	}
	if to != "" && replySubject != "" {
		command := "printf 'Hi there!' | hostbridge component " + componentRef +
			" messages send --to " + shellQuote(to) +
			" --subject " + shellQuote(replySubject)
		if threadID != "" {
			command += " --thread-id " + shellQuote(threadID)
		}
		if inReplyTo != "" {
			command += " --in-reply-to " + shellQuote(inReplyTo)
		}
		lines = append(lines, "Reply command:", command)
	}
	return strings.Join(lines, "\n")
}

func senderEmail(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	address, err := mail.ParseAddress(value)
	if err == nil && strings.TrimSpace(address.Address) != "" {
		return strings.TrimSpace(address.Address)
	}
	return value
}

func replySubject(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(subject), "re:") {
		return subject
	}
	return "Re: " + subject
}

func shellQuote(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
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
