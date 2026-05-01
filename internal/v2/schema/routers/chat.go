package routers

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	v2commands "github.com/bartdeboer/ctgbot/internal/v2/schema/commands"
)

type ChatHandlers interface {
	ListUnregisteredChats(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
	ApplyChatPreset(ctx context.Context, req commandengine.Request, cmd v2commands.ChatApplyPreset) (commandengine.Result, error)
}

func RegisterChatHandlers(registry *commandengine.Registry, handlers ChatHandlers) error {
	if handlers == nil {
		return fmt.Errorf("missing chat command handlers")
	}
	if err := commandengine.Register[v2commands.ChatListUnregistered](registry, func(ctx context.Context, req commandengine.Request, cmd v2commands.ChatListUnregistered) (commandengine.Result, error) {
		return handlers.ListUnregisteredChats(ctx, req)
	}); err != nil {
		return err
	}
	return commandengine.Register[v2commands.ChatApplyPreset](registry, func(ctx context.Context, req commandengine.Request, cmd v2commands.ChatApplyPreset) (commandengine.Result, error) {
		return handlers.ApplyChatPreset(ctx, req, cmd)
	})
}
