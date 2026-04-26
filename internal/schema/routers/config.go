package routers

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configengine"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

type ConfigHandlers interface {
	ScaffoldHostbridgeAllowedCommand(ctx context.Context, req commandengine.Request, cmd schemacommands.ConfigHostbridgeScaffold) (commandengine.Result, error)
}

func NewConfigCommandEngine(manager *configengine.Manager, source commandengine.Source, handlers ...ConfigHandlers) (*commandengine.Engine, error) {
	router, err := commandengine.NewRouter(schemacommands.ConfigCommands(), source)
	if err != nil {
		return nil, err
	}
	registry := commandengine.NewRegistry()
	if err := RegisterConfigHandlers(registry, manager, handlers...); err != nil {
		return nil, err
	}
	return commandengine.NewEngine(router, registry), nil
}

func RegisterConfigHandlers(registry *commandengine.Registry, manager *configengine.Manager, handlers ...ConfigHandlers) error {
	if manager == nil {
		return fmt.Errorf("missing config manager")
	}
	if err := commandengine.Register[schemacommands.ConfigList](registry, func(_ context.Context, req commandengine.Request, cmd schemacommands.ConfigList) (commandengine.Result, error) {
		items := manager.List(req.Context)
		names := make([]string, 0, len(items))
		for _, item := range items {
			names = append(names, item.Name())
		}
		return commandengine.Result{Text: strings.Join(names, "\n")}, nil
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.ConfigGet](registry, func(_ context.Context, req commandengine.Request, cmd schemacommands.ConfigGet) (commandengine.Result, error) {
		value, err := manager.Get(req.Context, cmd.Key)
		if err != nil {
			return commandengine.Result{}, err
		}
		return commandengine.Result{Text: configResult(cmd.Key, value)}, nil
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.ConfigSet](registry, func(_ context.Context, req commandengine.Request, cmd schemacommands.ConfigSet) (commandengine.Result, error) {
		value, err := manager.Set(req.Context, cmd.Key, cmd.Value)
		if err != nil {
			return commandengine.Result{}, err
		}
		return commandengine.Result{Text: configResult(cmd.Key, value)}, nil
	}); err != nil {
		return err
	}
	handler := firstConfigHandler(handlers)
	if handler == nil {
		return nil
	}
	return commandengine.Register[schemacommands.ConfigHostbridgeScaffold](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.ConfigHostbridgeScaffold) (commandengine.Result, error) {
		return handler.ScaffoldHostbridgeAllowedCommand(ctx, req, cmd)
	})
}

func firstConfigHandler(handlers []ConfigHandlers) ConfigHandlers {
	for _, handler := range handlers {
		if handler != nil {
			return handler
		}
	}
	return nil
}

func configResult(key string, value configengine.Value) string {
	return configengine.NormalizeKey(key) + "=" + value.String()
}
