package runtime

import (
	"context"
	"fmt"

	v2broker "github.com/bartdeboer/ctgbot/internal/v2/broker"
	v2component "github.com/bartdeboer/ctgbot/internal/v2/component"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

type RunOptions struct {
	DefaultChatComponents []coremodel.ChatComponent
	RoleResolver          v2broker.RoleResolver
	EventErrorHandler     func(ctx context.Context, event v2component.InboundEvent, err error)
	Logf                  func(format string, args ...any)
}

func Run(ctx context.Context, rt *Runtime, registry *v2component.Registry, opts RunOptions) error {
	if rt == nil {
		return fmt.Errorf("missing v2 runtime")
	}
	if rt.Storage == nil {
		return fmt.Errorf("missing v2 runtime storage")
	}
	broker := v2broker.New(rt.Storage, registry)
	broker.DefaultChatComponents = opts.DefaultChatComponents
	broker.RoleResolver = opts.RoleResolver
	broker.EventErrorHandler = opts.EventErrorHandler
	broker.Logf = opts.Logf
	return broker.Run(ctx)
}
