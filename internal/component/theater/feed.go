package theater

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func (c *Component) pendingUpdates(ctx context.Context, subscriberThreadID modeluuid.UUID) ([]component.UpdateNotice, error) {
	if c == nil || c.store == nil || c.storage == nil || subscriberThreadID.IsNull() {
		return nil, nil
	}
	subscriptions, err := c.store.subscriptions(ctx, subscriberThreadID)
	if err != nil {
		return nil, err
	}
	out := make([]component.UpdateNotice, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		theater, err := c.store.theaterByID(ctx, subscription.TheaterID)
		if err != nil {
			return nil, err
		}
		theaterThreadID, ok, err := theaterThreadID(theater)
		if err != nil || !ok {
			if err != nil {
				return nil, err
			}
			continue
		}
		count, err := c.storage.Messages().CountByThreadIDSince(ctx, theaterThreadID, subscription.LastReadAt)
		if err != nil {
			return nil, err
		}
		if count <= 0 {
			continue
		}
		label := strings.TrimSpace(theater.Label)
		if label == "" {
			label = theater.Name
		}
		out = append(out, component.UpdateNotice{Source: Type, Ref: theater.Name, Label: label, Kind: "message", Count: int(count)})
	}
	return out, nil
}

func theaterThreadID(theater theaterRecord) (modeluuid.UUID, bool, error) {
	raw := strings.TrimSpace(theater.ThreadID)
	if raw == "" {
		return modeluuid.Nil, false, nil
	}
	threadID, err := modeluuid.Parse(raw)
	if err != nil {
		return modeluuid.Nil, false, fmt.Errorf("parse theater thread id: %w", err)
	}
	return threadID, true, nil
}
