package configsurface

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type ConfigList struct{}

type ConfigGet struct {
	Key string
}

type ConfigSet struct {
	Key   string
	Value string
}

type ConfigUnset struct {
	Key string
}

type DefinitionOptions struct {
	Sources []commandengine.Source
	Policy  simplerbac.Rule
}

func CommandDefinitions(options DefinitionOptions) []commandengine.Definition {
	return []commandengine.Definition{
		{
			Pattern:               "config list",
			Help:                  "List config keys, values, defaults, and options",
			Build:                 func(req *clir.Request) (any, error) { _ = req; return ConfigList{}, nil },
			Sources:               append([]commandengine.Source{}, options.Sources...),
			Policy:                options.Policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "config get <key>",
			Help:    "Show config value, default, and options for one key",
			Build: func(req *clir.Request) (any, error) {
				key := NormalizeKey(req.Params["key"])
				if key == "" {
					return nil, fmt.Errorf("missing config key")
				}
				return ConfigGet{Key: key}, nil
			},
			Sources:               append([]commandengine.Source{}, options.Sources...),
			Policy:                options.Policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "config set <key> <value>",
			Help:    "Set a config value",
			Build: func(req *clir.Request) (any, error) {
				key := NormalizeKey(req.Params["key"])
				if key == "" {
					return nil, fmt.Errorf("missing config key")
				}
				return ConfigSet{Key: key, Value: strings.TrimSpace(req.Params["value"])}, nil
			},
			Sources: append([]commandengine.Source{}, options.Sources...),
			Policy:  options.Policy,
		},
		{
			Pattern: "config unset <key>",
			Help:    "Remove a config override and fall back to the default",
			Build: func(req *clir.Request) (any, error) {
				key := NormalizeKey(req.Params["key"])
				if key == "" {
					return nil, fmt.Errorf("missing config key")
				}
				return ConfigUnset{Key: key}, nil
			},
			Sources: append([]commandengine.Source{}, options.Sources...),
			Policy:  options.Policy,
		},
	}
}

func RegisterGobTypes(register func(any)) {
	register(ConfigList{})
	register(ConfigGet{})
	register(ConfigSet{})
	register(ConfigUnset{})
}

func RegisterCommandHandlers(registry *commandengine.Registry, surface ConfigSurface) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if surface == nil {
		return fmt.Errorf("missing config surface")
	}
	if err := commandengine.RegisterPattern[ConfigList](registry, "config list", func(ctx context.Context, req commandengine.Request, cmd ConfigList) (commandengine.Result, error) {
		_ = cmd
		schema, err := surface.ConfigSchema(ctx, req)
		if err != nil {
			return commandengine.Result{}, err
		}
		return commandengine.Result{Text: FormatList(ctx, req, surface, schema)}, nil
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[ConfigGet](registry, "config get <key>", func(ctx context.Context, req commandengine.Request, cmd ConfigGet) (commandengine.Result, error) {
		_, field, err := fieldForCommand(ctx, req, surface, cmd.Key)
		if err != nil {
			return commandengine.Result{}, err
		}
		value, err := surface.ConfigGet(ctx, req, field.Key)
		if err != nil {
			return commandengine.Result{}, err
		}
		return commandengine.Result{Text: FormatGet(field, value)}, nil
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[ConfigSet](registry, "config set <key> <value>", func(ctx context.Context, req commandengine.Request, cmd ConfigSet) (commandengine.Result, error) {
		_, field, err := fieldForCommand(ctx, req, surface, cmd.Key)
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
	return commandengine.RegisterPattern[ConfigUnset](registry, "config unset <key>", func(ctx context.Context, req commandengine.Request, cmd ConfigUnset) (commandengine.Result, error) {
		_, field, err := fieldForCommand(ctx, req, surface, cmd.Key)
		if err != nil {
			return commandengine.Result{}, err
		}
		if !field.Writable {
			return commandengine.Result{}, fmt.Errorf("config %s is read-only", field.Key)
		}
		if err := surface.ConfigUnset(ctx, req, field.Key); err != nil {
			return commandengine.Result{}, err
		}
		value, err := surface.ConfigGet(ctx, req, field.Key)
		if err != nil {
			return commandengine.Result{}, err
		}
		return commandengine.Result{Text: field.Key + "=" + value}, nil
	})
}

func fieldForCommand(ctx context.Context, req commandengine.Request, surface ConfigSurface, key string) (ConfigSchema, FieldSchema, error) {
	schema, err := surface.ConfigSchema(ctx, req)
	if err != nil {
		return ConfigSchema{}, FieldSchema{}, err
	}
	field, ok := schema.Field(key)
	if !ok {
		return schema, FieldSchema{}, fmt.Errorf("unknown config key %q", NormalizeKey(key))
	}
	return schema, field, nil
}
