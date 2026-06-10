package theater

import (
	"context"
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
		targetThreadID, err := modeluuid.Parse(subscription.TargetThreadID)
		if err != nil {
			return nil, err
		}
		count, err := c.unreadCount(ctx, targetThreadID, subscription.LastReadAt)
		if err != nil {
			return nil, err
		}
		if count <= 0 {
			continue
		}
		label := strings.TrimSpace(subscription.Label)
		if label == "" {
			label = subscription.TargetThreadID
		}
		out = append(out, component.UpdateNotice{Source: Type, Ref: subscription.TargetThreadID, Label: label, Kind: "message", Count: int(count)})
	}
	return out, nil
}
