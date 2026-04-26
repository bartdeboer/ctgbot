package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func chatBool(key string, help string, cfg *appstate.Config, get func(appstate.ChatConfig) bool, set func(appstate.ChatConfig, bool) error, writePolicy simplerbac.Rule) configengine.Item {
	return configengine.Item{
		Key:         key,
		Help:        help,
		Scope:       configengine.ScopeChat,
		ValueType:   configengine.ValueBool,
		ReadPolicy:  anyOperator(),
		WritePolicy: writePolicy,
		Get: func(ctx commandengine.Context) (configengine.Value, error) {
			if cfg == nil {
				return configengine.Value{}, fmt.Errorf("missing config")
			}
			return configengine.String(strconv.FormatBool(get(cfg.Chat(ctx.ChatID)))), nil
		},
		Set: func(ctx commandengine.Context, value configengine.Value) (configengine.Value, error) {
			if cfg == nil {
				return configengine.Value{}, fmt.Errorf("missing config")
			}
			parsed, err := value.Bool()
			if err != nil {
				return configengine.Value{}, err
			}
			chat := cfg.Chat(ctx.ChatID)
			if err := set(chat, parsed); err != nil {
				return configengine.Value{}, err
			}
			return configengine.String(strconv.FormatBool(get(chat))), nil
		},
	}
}

func chatString(key string, help string, valueType configengine.ValueType, cfg *appstate.Config, get func(appstate.ChatConfig) string, set func(appstate.ChatConfig, string) error, writePolicy simplerbac.Rule) configengine.Item {
	return configengine.Item{
		Key:         key,
		Help:        help,
		Scope:       configengine.ScopeChat,
		ValueType:   valueType,
		ReadPolicy:  anyOperator(),
		WritePolicy: writePolicy,
		Get: func(ctx commandengine.Context) (configengine.Value, error) {
			if cfg == nil {
				return configengine.Value{}, fmt.Errorf("missing config")
			}
			return configengine.String(get(cfg.Chat(ctx.ChatID))), nil
		},
		Set: func(ctx commandengine.Context, value configengine.Value) (configengine.Value, error) {
			if cfg == nil {
				return configengine.Value{}, fmt.Errorf("missing config")
			}
			chat := cfg.Chat(ctx.ChatID)
			if err := set(chat, value.String()); err != nil {
				return configengine.Value{}, err
			}
			return configengine.String(get(chat)), nil
		},
	}
}

func chatStringList(key string, help string, cfg *appstate.Config, get func(appstate.ChatConfig) []string, set func(appstate.ChatConfig, []string) error, writePolicy simplerbac.Rule) configengine.Item {
	return configengine.Item{
		Key:         key,
		Help:        help,
		Scope:       configengine.ScopeChat,
		ValueType:   configengine.ValueStringList,
		ReadPolicy:  anyOperator(),
		WritePolicy: writePolicy,
		Get: func(ctx commandengine.Context) (configengine.Value, error) {
			if cfg == nil {
				return configengine.Value{}, fmt.Errorf("missing config")
			}
			return configengine.List(get(cfg.Chat(ctx.ChatID))), nil
		},
		Set: func(ctx commandengine.Context, value configengine.Value) (configengine.Value, error) {
			if cfg == nil {
				return configengine.Value{}, fmt.Errorf("missing config")
			}
			chat := cfg.Chat(ctx.ChatID)
			if err := set(chat, value.StringList()); err != nil {
				return configengine.Value{}, err
			}
			return configengine.List(get(chat)), nil
		},
	}
}

func rootString(key string, help string, valueType configengine.ValueType, cfg *appstate.Config, get func(*appstate.Config) string, set func(*appstate.Config, string) error, readPolicy simplerbac.Rule, writePolicy simplerbac.Rule) configengine.Item {
	item := configengine.Item{
		Key:         key,
		Help:        help,
		Scope:       configengine.ScopeRoot,
		ValueType:   valueType,
		ReadPolicy:  readPolicy,
		WritePolicy: writePolicy,
	}
	if get != nil {
		item.Get = func(ctx commandengine.Context) (configengine.Value, error) {
			if cfg == nil {
				return configengine.Value{}, fmt.Errorf("missing config")
			}
			return configengine.String(get(cfg)), nil
		}
	}
	if set != nil {
		item.Set = func(ctx commandengine.Context, value configengine.Value) (configengine.Value, error) {
			if cfg == nil {
				return configengine.Value{}, fmt.Errorf("missing config")
			}
			if err := set(cfg, value.String()); err != nil {
				return configengine.Value{}, err
			}
			if get == nil {
				return configengine.String(value.String()), nil
			}
			return configengine.String(get(cfg)), nil
		}
	}
	return item
}

func rootInt64(key string, help string, cfg *appstate.Config, get func(*appstate.Config) int64, set func(*appstate.Config, int64) error, readPolicy simplerbac.Rule, writePolicy simplerbac.Rule) configengine.Item {
	return rootString(key, help, configengine.ValueInt, cfg,
		func(cfg *appstate.Config) string { return strconv.FormatInt(get(cfg), 10) },
		func(cfg *appstate.Config, value string) error {
			parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
			if err != nil {
				return fmt.Errorf("invalid int %q", value)
			}
			return set(cfg, parsed)
		},
		readPolicy,
		writePolicy,
	)
}

func rootReadOnlyInt(key string, help string, value int64, readPolicy simplerbac.Rule) configengine.Item {
	return configengine.Item{
		Key:         key,
		Help:        help,
		Scope:       configengine.ScopeRoot,
		ValueType:   configengine.ValueInt,
		ReadPolicy:  readPolicy,
		WritePolicy: rootOnly(),
		Get: func(ctx commandengine.Context) (configengine.Value, error) {
			return configengine.String(strconv.FormatInt(value, 10)), nil
		},
	}
}

func anyOperator() simplerbac.Rule {
	return simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser)
}

func rootOrAgent() simplerbac.Rule {
	return simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent)
}

func rootOrElevated() simplerbac.Rule {
	return simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleElevated)
}

func rootAgentOrElevated() simplerbac.Rule {
	return simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleElevated)
}

func rootOnly() simplerbac.Rule {
	return simplerbac.Any(simplerbac.RoleRoot)
}
