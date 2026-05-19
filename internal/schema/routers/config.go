package routers

import (
	"context"
	"fmt"

	globalconfig "github.com/bartdeboer/ctgbot/internal/app/config/global"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configengine"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
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
	surface := globalconfig.New(manager)
	if err := commandengine.Register[schemacommands.ConfigList](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.ConfigList) (commandengine.Result, error) {
		_ = cmd
		schema, err := surface.ConfigSchema(ctx, req)
		if err != nil {
			return commandengine.Result{}, err
		}
		return commandengine.Result{Text: configsurface.FormatList(ctx, req, surface, schema)}, nil
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.ConfigGet](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.ConfigGet) (commandengine.Result, error) {
		_, field, err := globalConfigField(ctx, req, surface, cmd.Key)
		if err != nil {
			return commandengine.Result{}, err
		}
		value, err := surface.ConfigGet(ctx, req, field.Key)
		if err != nil {
			return commandengine.Result{}, err
		}
		return commandengine.Result{Text: configsurface.FormatGet(field, value)}, nil
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.ConfigSet](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.ConfigSet) (commandengine.Result, error) {
		_, field, err := globalConfigField(ctx, req, surface, cmd.Key)
		if err != nil {
			return commandengine.Result{}, err
		}
		if !field.Writable {
			return commandengine.Result{}, fmt.Errorf("config %s is read-only", field.Key)
		}
		if err := surface.ConfigSet(ctx, req, field.Key, cmd.Value); err != nil {
			return commandengine.Result{}, err
		}
		value, err := surface.ConfigGet(ctx, req, field.Key)
		if err != nil {
			return commandengine.Result{}, err
		}
		return commandengine.Result{Text: field.Key + "=" + value}, nil
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

func globalConfigField(ctx context.Context, req commandengine.Request, surface configsurface.ConfigSurface, key string) (configsurface.ConfigSchema, configsurface.FieldSchema, error) {
	schema, err := surface.ConfigSchema(ctx, req)
	if err != nil {
		return configsurface.ConfigSchema{}, configsurface.FieldSchema{}, err
	}
	field, ok := schema.Field(key)
	if !ok {
		return schema, configsurface.FieldSchema{}, fmt.Errorf("unknown config key %q", configsurface.NormalizeKey(key))
	}
	return schema, field, nil
}

func firstConfigHandler(handlers []ConfigHandlers) ConfigHandlers {
	for _, handler := range handlers {
		if handler != nil {
			return handler
		}
	}
	return nil
}
