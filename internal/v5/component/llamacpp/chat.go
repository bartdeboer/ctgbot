package llamacpp

import (
	"strings"

	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func brokerMessagesToChat(messages []coremodel.ThreadMessage) []chatMessage {
	out := make([]chatMessage, 0, len(messages))
	for _, message := range messages {
		content := strings.TrimSpace(message.Text)
		if content == "" {
			continue
		}
		role := roleForMessage(message)
		if role == "" {
			continue
		}
		out = append(out, chatMessage{Role: role, Content: content})
	}
	return out
}

func roleForMessage(message coremodel.ThreadMessage) string {
	switch message.Kind {
	case coremodel.MessageKindSystem:
		return "system"
	case coremodel.MessageKindAgent:
		return "assistant"
	case coremodel.MessageKindUser:
		return "user"
	}
	switch message.Direction {
	case coremodel.MessageDirectionInbound:
		return "user"
	case coremodel.MessageDirectionOutbound:
		return "assistant"
	}
	return ""
}
