package outlook

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/component"
)

func (c *Component) RunInbound(ctx context.Context, emit component.InboundEmitter) error {
	if emit == nil {
		return fmt.Errorf("missing inbound emitter")
	}
	interval := c.componentConfig.pollInterval()
	client, err := c.client(ctx)
	if err != nil {
		return err
	}
	for {
		if err := c.pollOnce(ctx, client, emit); err != nil {
			c.logf("outlook poll failed component=%s err=%v", c.registration.Ref(), err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

func (c *Component) pollOnce(ctx context.Context, client outlookClient, emit component.InboundEmitter) error {
	state, _ := c.loadState()
	seen := map[string]bool{}
	for _, id := range state.SeenIDs {
		seen[strings.TrimSpace(id)] = true
	}
	messages, err := client.ListMessages(ctx, c.componentConfig.PollFolder, c.componentConfig.MaxPollMessages)
	if err != nil {
		return err
	}
	if state.LastSyncedAt.IsZero() {
		for _, m := range messages {
			if strings.TrimSpace(m.ID) != "" {
				seen[strings.TrimSpace(m.ID)] = true
			}
		}
		state.SeenIDs = mapKeys(seen)
		state.LastSyncedAt = time.Now().UTC()
		return c.saveState(state)
	}
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		id := strings.TrimSpace(m.ID)
		if id == "" || seen[id] {
			continue
		}
		record, _, err := c.storeIncomingMessage(ctx, m)
		if err != nil {
			return err
		}
		if err := emit(ctx, c.inboundEventFromStoredMessage(*record)); err != nil {
			return err
		}
		seen[id] = true
	}
	state.SeenIDs = mapKeys(seen)
	state.LastSyncedAt = time.Now().UTC()
	return c.saveState(state)
}
func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		if strings.TrimSpace(k) != "" {
			out = append(out, k)
		}
	}
	if len(out) > 500 {
		out = out[len(out)-500:]
	}
	return out
}
