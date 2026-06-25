package llamacppagent

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/component"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

var _ component.ThreadSandboxProvider = (*Component)(nil)

func (c *Component) ThreadSandbox(ctx context.Context, request component.ThreadSandboxRequest) (*sandboxengine.Sandbox, error) {
	if c == nil || c.runtime == nil {
		return nil, fmt.Errorf("missing llamacppagent runtime")
	}
	provider, ok := c.runtime.(runtimepkg.ThreadSandboxProvider)
	if !ok {
		return nil, fmt.Errorf("runtime does not expose thread sandbox: %T", c.runtime)
	}
	return provider.ThreadSandbox(ctx, request.WorkspacePath, request.Thread.ID)
}
