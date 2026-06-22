package gmailv2

import (
	"bytes"
	"fmt"
	"mime"
	"net/mail"
	"strings"
)

const maxReferencesHeaderBytes = 900

type rawMessageHeaders struct {
	MessageID  string
	References string
	InReplyTo  string
	Subject    string
	From       string
	ReplyTo    string
	To         string
	Cc         string
}

func parseRawMessageHeaders(raw []byte) (rawMessageHeaders, error) {
	message, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return rawMessageHeaders{}, fmt.Errorf("parse raw message headers: %w", err)
	}
	headers := rawMessageHeaders{
		MessageID:  normalizeMessageID(message.Header.Get("Message-ID")),
		References: strings.TrimSpace(message.Header.Get("References")),
		InReplyTo:  normalizeMessageID(message.Header.Get("In-Reply-To")),
		Subject:    decodeHeaderText(message.Header.Get("Subject")),
		From:       strings.TrimSpace(message.Header.Get("From")),
		ReplyTo:    strings.TrimSpace(message.Header.Get("Reply-To")),
		To:         strings.TrimSpace(message.Header.Get("To")),
		Cc:         strings.TrimSpace(message.Header.Get("Cc")),
	}
	return headers, nil
}

func appendReferenceMessageID(previous string, messageID string) string {
	ids := referenceMessageIDs(previous)
	messageID = normalizeMessageID(messageID)
	if messageID != "" && !containsFold(ids, messageID) {
		ids = append(ids, messageID)
	}
	for len(strings.Join(ids, " ")) > maxReferencesHeaderBytes && len(ids) > 1 {
		ids = ids[1:]
	}
	return strings.Join(ids, " ")
}

func referenceMessageIDs(value string) []string {
	fields := strings.Fields(strings.TrimSpace(value))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		id := normalizeMessageID(field)
		if id != "" && !containsFold(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func normalizeMessageID(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), "")
	value = strings.Trim(value, "<>")
	if value == "" {
		return ""
	}
	return "<" + value + ">"
}

func decodeHeaderText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	decoded, err := new(mime.WordDecoder).DecodeHeader(value)
	if err != nil {
		return value
	}
	return strings.TrimSpace(decoded)
}

func containsFold(values []string, needle string) bool {
	for _, value := range values {
		if strings.EqualFold(value, needle) {
			return true
		}
	}
	return false
}
