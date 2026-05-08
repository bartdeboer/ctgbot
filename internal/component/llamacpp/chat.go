package llamacpp

import (
	"strings"

	component "github.com/bartdeboer/ctgbot/internal/component"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func completionPromptToChat(prompt component.CompletionPrompt) []chatMessage {
	out := make([]chatMessage, 0, len(prompt.Messages))
	for _, message := range prompt.Messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		role := strings.TrimSpace(string(message.Role))
		if role == "" {
			continue
		}
		out = append(out, chatMessage{Role: role, Content: content})
	}
	return out
}
