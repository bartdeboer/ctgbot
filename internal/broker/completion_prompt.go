package broker

import (
	"context"
	"strings"

	component "github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func (b *Broker) completionPrompt(ctx context.Context, threadID modeluuid.UUID, inbound coremodel.ThreadMessage) (component.CompletionPrompt, error) {
	messages, err := b.Storage.Messages().ListByThreadID(ctx, threadID)
	if err != nil {
		return component.CompletionPrompt{}, err
	}
	for i := range messages {
		if messages[i].ID == inbound.ID {
			messages[i] = inbound
		}
	}
	return completionPromptFromMessages(messages), nil
}

func completionPromptFromMessages(messages []coremodel.ThreadMessage) component.CompletionPrompt {
	prompt := component.CompletionPrompt{
		Messages: make([]component.CompletionMessage, 0, len(messages)),
	}
	for _, message := range messages {
		content := strings.TrimSpace(message.Text)
		if content == "" {
			continue
		}
		role, ok := completionRoleForMessage(message)
		if !ok {
			continue
		}
		prompt.Messages = append(prompt.Messages, component.CompletionMessage{
			Role:    role,
			Content: content,
		})
	}
	return prompt
}

func completionRoleForMessage(message coremodel.ThreadMessage) (component.CompletionRole, bool) {
	switch message.Kind {
	case coremodel.MessageKindSystem:
		return component.CompletionRoleSystem, true
	case coremodel.MessageKindAgent:
		return component.CompletionRoleAssistant, true
	case coremodel.MessageKindUser:
		return component.CompletionRoleUser, true
	}
	switch message.Direction {
	case coremodel.MessageDirectionInbound:
		return component.CompletionRoleUser, true
	case coremodel.MessageDirectionOutbound:
		return component.CompletionRoleAssistant, true
	default:
		return "", false
	}
}
