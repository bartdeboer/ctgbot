package routers

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

type ThreadHandlers interface {
	RefreshContainer(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
	StartContainer(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
	StopContainer(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
	PurgeChat(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
	InterruptTurn(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
	Install(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
	Upgrade(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
	Quit(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
	Status(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
	ModelStatus(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
	ModelList(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
	ModelSet(ctx context.Context, req commandengine.Request, cmd schemacommands.ModelSet) (commandengine.Result, error)
	ModelClear(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
	ModelEffortStatus(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
	ModelEffortList(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
	ModelEffortSet(ctx context.Context, req commandengine.Request, cmd schemacommands.ModelEffortSet) (commandengine.Result, error)
	ModelEffortClear(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
}

func RegisterThreadHandlers(registry *commandengine.Registry, handlers ThreadHandlers) error {
	if handlers == nil {
		return fmt.Errorf("missing thread command handlers")
	}
	if err := commandengine.Register[schemacommands.RefreshContainer](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.RefreshContainer) (commandengine.Result, error) {
		return handlers.RefreshContainer(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.StartContainer](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.StartContainer) (commandengine.Result, error) {
		return handlers.StartContainer(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.StopContainer](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.StopContainer) (commandengine.Result, error) {
		return handlers.StopContainer(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.PurgeChat](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.PurgeChat) (commandengine.Result, error) {
		return handlers.PurgeChat(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.InterruptTurn](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.InterruptTurn) (commandengine.Result, error) {
		return handlers.InterruptTurn(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.Install](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.Install) (commandengine.Result, error) {
		return handlers.Install(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.Upgrade](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.Upgrade) (commandengine.Result, error) {
		return handlers.Upgrade(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.Quit](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.Quit) (commandengine.Result, error) {
		return handlers.Quit(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.Status](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.Status) (commandengine.Result, error) {
		return handlers.Status(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.ModelStatus](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.ModelStatus) (commandengine.Result, error) {
		return handlers.ModelStatus(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.ModelList](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.ModelList) (commandengine.Result, error) {
		return handlers.ModelList(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.ModelSet](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.ModelSet) (commandengine.Result, error) {
		return handlers.ModelSet(ctx, req, cmd)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.ModelClear](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.ModelClear) (commandengine.Result, error) {
		return handlers.ModelClear(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.ModelEffortStatus](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.ModelEffortStatus) (commandengine.Result, error) {
		return handlers.ModelEffortStatus(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.ModelEffortList](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.ModelEffortList) (commandengine.Result, error) {
		return handlers.ModelEffortList(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.ModelEffortSet](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.ModelEffortSet) (commandengine.Result, error) {
		return handlers.ModelEffortSet(ctx, req, cmd)
	}); err != nil {
		return err
	}
	return commandengine.Register[schemacommands.ModelEffortClear](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.ModelEffortClear) (commandengine.Result, error) {
		return handlers.ModelEffortClear(ctx, req)
	})
}
