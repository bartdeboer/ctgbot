package outlook

import (
	"context"
	"fmt"
	"strings"
)

func (c *Component) storeIncomingMessage(ctx context.Context, message graphMessage) (*storedMessage, bool, error) {
	id := strings.TrimSpace(message.ID)
	if id == "" {
		return nil, false, fmt.Errorf("missing outlook message id")
	}
	existing, err := c.store.messageByOutlookID(ctx, id)
	if err != nil || existing != nil {
		return existing, false, err
	}
	record := c.parseMessage(message)
	record.ID = newStoreID()
	if err := c.store.saveMessage(ctx, &record); err != nil {
		return nil, false, err
	}
	return &record, true, nil
}
