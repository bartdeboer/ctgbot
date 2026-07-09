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
	client, err := c.waitForClient(ctx, interval)
	if err != nil {
		return err
	}
	for {
		if err := c.pollOnce(ctx, client, emit); err != nil {
			c.logf("outlook poll failed component=%s err=%v", c.registration.Ref(), err)
		}
		if err := waitInterval(ctx, interval); err != nil {
			return err
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

func (c *Component) waitForClient(ctx context.Context, interval time.Duration) (outlookClient, error) {
	logged := false
	for {
		client, err := c.client(ctx)
		if err == nil {
			if logged {
				c.logf("outlook source authenticated component=%s", c.registration.Ref())
			}
			return client, nil
		}
		if !isMissingAuthMaterial(err) {
			return nil, err
		}
		if !logged {
			c.logf("outlook source unavailable component=%s err=%v", c.registration.Ref(), err)
			logged = true
		}
		if waitErr := waitInterval(ctx, interval); waitErr != nil {
			return nil, waitErr
		}
	}
}

func waitInterval(ctx context.Context, interval time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isMissingAuthMaterial(err error) bool {
	return err != nil && strings.Contains(err.Error(), errMissingAuthMaterial.Error())
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
