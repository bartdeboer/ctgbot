package theater

import (
	"context"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

var _ component.TurnHandler = (*Component)(nil)

func (c *Component) HandleTurn(ctx context.Context, turn component.Turn) (*component.TurnResult, error) {
	_ = ctx
	if !isInternalThreadPost(turn.Inbound) {
		return nil, nil
	}
	text := strings.TrimSpace(turn.Inbound.Text)
	if text == "" {
		return nil, nil
	}
	return &component.TurnResult{Final: &coremodel.ThreadMessage{
		Role:         coremodel.MessageRoleAgent,
		Kind:         coremodel.MessageKindMessage,
		ComponentID:  c.registration.ID,
		ActorID:      Type,
		ActorLabel:   "theater",
		Text:         formatRelayText(turn.Inbound),
		MetadataJSON: "relay=theater",
	}}, nil
}

func isInternalThreadPost(message coremodel.ThreadMessage) bool {
	return metadataValue(message.MetadataJSON, "provider") == "thread"
}

func formatRelayText(message coremodel.ThreadMessage) string {
	text := strings.TrimSpace(message.Text)
	label := strings.TrimSpace(message.ActorLabel)
	if label == "" {
		label = strings.TrimSpace(message.ActorID)
	}
	if label == "" {
		return text
	}
	if strings.Contains(text, "\n") {
		return label + ":\n" + text
	}
	return label + ": " + text
}

func metadataValue(metadata string, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	prefix := key + "="
	for _, line := range strings.Split(metadata, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}
