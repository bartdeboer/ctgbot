package theater

import (
	"context"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

func (c *Component) HandleTurn(ctx context.Context, turn component.Turn) (*component.TurnResult, error) {
	_ = ctx
	if !isInternalProvider(turn.Inbound.ProviderType) {
		return nil, nil
	}
	text := strings.TrimSpace(turn.Inbound.Text)
	if text == "" {
		return nil, nil
	}
	return &component.TurnResult{Final: &coremodel.ThreadMessage{
		Role:       coremodel.MessageRoleAgent,
		Kind:       coremodel.MessageKindMessage,
		ActorID:    strings.TrimSpace(turn.Inbound.ActorID),
		ActorLabel: strings.TrimSpace(turn.Inbound.ActorLabel),
		Text:       text,
	}}, nil
}

func isInternalProvider(provider string) bool {
	switch strings.TrimSpace(provider) {
	case "thread", "theater":
		return true
	default:
		return false
	}
}
