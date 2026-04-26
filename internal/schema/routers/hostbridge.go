package routers

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configengine"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

type HostbridgeHandlers interface {
	RunCommand(ctx context.Context, req commandengine.Request, cmd schemacommands.RunCommand) (commandengine.Result, error)
	SendMedia(ctx context.Context, req commandengine.Request, cmd schemacommands.SendMedia) (commandengine.Result, error)
}

func HostbridgeDefinitions() []commandengine.Definition {
	out := append([]commandengine.Definition{}, schemacommands.ConfigCommands()...)
	out = append(out, schemacommands.ThreadCommands()...)
	out = append(out, schemacommands.HostbridgeCommands()...)
	return definitionsForSource(out, commandengine.SourceHostbridge)
}

func HostbridgeRunDefinitions() []commandengine.Definition {
	return []commandengine.Definition{schemacommands.RunCommandDefinition()}
}

func NewHostbridgeRouter() (*commandengine.Router, error) {
	return commandengine.NewRouter(HostbridgeDefinitions(), commandengine.SourceHostbridge)
}

func NewHostbridgeRunRouter() (*commandengine.Router, error) {
	return commandengine.NewRouter(HostbridgeRunDefinitions(), commandengine.SourceHostbridge)
}

func NewHostbridgeCommandEngine(manager *configengine.Manager, threadHandlers ThreadHandlers, hostbridgeHandlers HostbridgeHandlers) (*commandengine.Engine, error) {
	router, err := NewHostbridgeRouter()
	if err != nil {
		return nil, err
	}
	registry := commandengine.NewRegistry()
	if err := RegisterConfigHandlers(registry, manager, configHandlerFrom(threadHandlers), configHandlerFrom(hostbridgeHandlers)); err != nil {
		return nil, err
	}
	if err := RegisterThreadHandlers(registry, threadHandlers); err != nil {
		return nil, err
	}
	if err := RegisterHostbridgeHandlers(registry, hostbridgeHandlers); err != nil {
		return nil, err
	}
	return commandengine.NewEngine(router, registry), nil
}

func configHandlerFrom(candidate any) ConfigHandlers {
	handler, _ := candidate.(ConfigHandlers)
	return handler
}

func RegisterHostbridgeHandlers(registry *commandengine.Registry, handlers HostbridgeHandlers) error {
	if handlers == nil {
		return fmt.Errorf("missing hostbridge command handlers")
	}
	if err := commandengine.Register[schemacommands.RunCommand](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.RunCommand) (commandengine.Result, error) {
		return handlers.RunCommand(ctx, req, cmd)
	}); err != nil {
		return err
	}
	return commandengine.Register[schemacommands.SendMedia](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.SendMedia) (commandengine.Result, error) {
		return handlers.SendMedia(ctx, req, cmd)
	})
}
