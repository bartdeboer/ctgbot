package codex

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/component"
)

var _ component.ThreadSandboxKeepRunning = (*Component)(nil)

func (c *Component) ThreadSandboxKeepRunning(ctx context.Context, request component.ThreadSandboxRequest) (bool, error) {
	settings, err := c.resolveThreadSettings(ctx, &request.Thread)
	if err != nil {
		return false, err
	}
	return settings.KeepRunning, nil
}

func (c *Component) SetThreadSandboxKeepRunning(ctx context.Context, request component.ThreadSandboxRequest, keepRunning *bool) error {
	return c.SetKeepRunning(ctx, &request.Thread, keepRunning)
}
