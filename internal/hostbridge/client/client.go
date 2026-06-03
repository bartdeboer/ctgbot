package client

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/hostbridge/transport"
)

// Client runs typed hostbridge commands through the configured command runner.
// It does not know which codec or wire transport is used underneath.
type Client struct {
	Runner transport.CommandRunner
}

func New(runner transport.CommandRunner) *Client {
	return &Client{Runner: runner}
}

func (c *Client) DoCommand(ctx context.Context, req hostbridge.CommandRequest) (hostbridge.CommandResponse, error) {
	if c == nil || c.Runner == nil {
		return hostbridge.CommandResponse{}, fmt.Errorf("missing hostbridge command runner")
	}
	return c.Runner.RunCommand(ctx, req)
}
