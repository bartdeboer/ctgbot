package routers

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	v2commands "github.com/bartdeboer/ctgbot/internal/v2/schema/commands"
)

type ProcessHandlers interface {
	Install(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
	Quit(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
}

func RegisterProcessHandlers(registry *commandengine.Registry, handlers ProcessHandlers) error {
	if handlers == nil {
		return fmt.Errorf("missing process command handlers")
	}
	if err := commandengine.Register[v2commands.Install](registry, func(ctx context.Context, req commandengine.Request, cmd v2commands.Install) (commandengine.Result, error) {
		return handlers.Install(ctx, req)
	}); err != nil {
		return err
	}
	return commandengine.Register[v2commands.Quit](registry, func(ctx context.Context, req commandengine.Request, cmd v2commands.Quit) (commandengine.Result, error) {
		return handlers.Quit(ctx, req)
	})
}
