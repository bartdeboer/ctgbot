package claude

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/component"
)

var _ component.ThreadRuntimeController = (*Component)(nil)

func (c *Component) RefreshThreadRuntime(ctx context.Context, request component.ThreadRuntimeControlRequest) error {
	if c == nil || c.runtime == nil {
		return fmt.Errorf("missing claude runtime")
	}
	return c.runtime.Refresh(ctx, request.WorkspacePath, request.Thread.ID)
}
