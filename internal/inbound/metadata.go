package inbound

import (
	"net/mail"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/message"
)

func SenderIdentity(payload message.InboundPayload) (string, string) {
	actor := payload.ResolvedActor()
	label := strings.TrimSpace(actor.Label)
	if label == "" {
		label = strings.TrimSpace(actor.ID)
	}
	key := SenderKey(actor.ID)
	if key == "" {
		key = SenderKey(actor.Label)
	}
	if key == "" {
		key = "unknown"
	}
	if label == "" {
		label = key
	}
	return key, label
}

func SenderKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if address, err := mail.ParseAddress(value); err == nil && address != nil {
		return strings.ToLower(strings.TrimSpace(address.Address))
	}
	return strings.ToLower(value)
}

func SubjectFromText(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < len("Subject:") || !strings.EqualFold(line[:len("Subject:")], "Subject:") {
			continue
		}
		return strings.TrimSpace(line[len("Subject:"):])
	}
	return ""
}

func Preview(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	return text[:max]
}
