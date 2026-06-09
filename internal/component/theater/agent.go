package theater

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func (c *Component) HandleTurn(ctx context.Context, turn component.Turn) (*component.TurnResult, error) {
	if c == nil || c.store == nil {
		return nil, fmt.Errorf("missing theater store")
	}
	text := strings.TrimSpace(turn.Inbound.Text)
	if text == "" {
		return nil, nil
	}
	theater, ok, err := c.store.theaterForThread(ctx, turn.Thread.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	actorID, actorLabel := inboundActor(turn.Inbound)
	record, err := c.store.post(ctx, theater, turn.Thread.ID, actorID, actorLabel, text)
	if err != nil {
		return nil, err
	}
	if err := c.relayMessage(ctx, theater, record, turn.Thread.ID); err != nil {
		return nil, err
	}
	return nil, nil
}

func inboundActor(inbound coremodel.ThreadMessage) (string, string) {
	actorID := strings.TrimSpace(inbound.ActorID)
	actorLabel := strings.TrimSpace(inbound.ActorLabel)
	if actorLabel == "" {
		actorLabel = actorID
	}
	return actorID, actorLabel
}

func (c *Component) relayMessage(ctx context.Context, theater theaterRecord, record messageRecord, origin modeluuid.UUID) error {
	if c == nil || c.sender == nil {
		return nil
	}
	threadIDs, err := c.store.boundThreads(ctx, theater)
	if err != nil {
		return err
	}
	text := formatRelayMessage(theater, record)
	for _, threadID := range threadIDs {
		if !origin.IsNull() && threadID == origin {
			continue
		}
		if err := c.sender.SendPayload(ctx, threadID, message.OutboundPayload{
			Role: coremodel.MessageRoleAgent,
			Kind: coremodel.MessageKindMessage,
			Text: message.TextMessage{Text: text},
		}); err != nil {
			return err
		}
	}
	return nil
}

func formatRelayMessage(theater theaterRecord, record messageRecord) string {
	label := strings.TrimSpace(record.ActorLabel)
	if label == "" {
		label = strings.TrimSpace(record.ActorID)
	}
	if label == "" {
		label = "unknown"
	}
	return fmt.Sprintf("theater: %s\n%s: %s", theater.Name, label, strings.TrimSpace(record.Text))
}
