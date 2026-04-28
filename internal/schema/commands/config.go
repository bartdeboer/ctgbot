package commands

import (
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

type ConfigHostbridgeScaffold struct {
	Alias string
}

func ConfigCommands() []commandengine.Definition {
	return []commandengine.Definition{
		{
			ID:      "config.list",
			Sources: allSources(),
			Policy:  anyOperator(),
			Routes: []commandengine.Route{{
				Pattern: "config list",
				Help:    "List available config keys",
				Build:   func(req *clir.Request) (any, error) { return ConfigList{}, nil },
			}},
		},
		{
			ID:      "config.get",
			Sources: allSources(),
			Policy:  anyOperator(),
			Routes: []commandengine.Route{{
				Pattern: "config get <key>",
				Help:    "Show a config value",
				Build: func(req *clir.Request) (any, error) {
					key := strings.TrimSpace(req.Params["key"])
					if key == "" {
						return nil, fmt.Errorf("missing config key")
					}
					return ConfigGet{Key: key}, nil
				},
			}},
		},
		{
			ID:      "config.set",
			Sources: allSources(),
			Policy:  anyOperator(),
			Routes: []commandengine.Route{{
				Pattern: "config set <key> <value>",
				Help:    "Update a config value",
				Build: func(req *clir.Request) (any, error) {
					key := strings.TrimSpace(req.Params["key"])
					if key == "" {
						return nil, fmt.Errorf("missing config key")
					}
					return ConfigSet{Key: key, Value: req.Params["value"]}, nil
				},
			}},
		},
		{
			ID:      "config.hostbridge.scaffold",
			Sources: []commandengine.Source{commandengine.SourceCLI},
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleElevated),
			Routes: []commandengine.Route{{
				Pattern: "config hostbridge scaffold <alias>",
				Help:    "Create an editable hostbridge allowed-command skeleton",
				Build: func(req *clir.Request) (any, error) {
					alias := strings.TrimSpace(req.Params["alias"])
					if alias == "" {
						return nil, fmt.Errorf("missing hostbridge alias")
					}
					return ConfigHostbridgeScaffold{Alias: alias}, nil
				},
			}},
		},
	}
}

func allSources() []commandengine.Source {
	return []commandengine.Source{
		commandengine.SourceCLI,
		commandengine.SourceMessage,
		commandengine.SourceHostbridge,
	}
}

func anyOperator() simplerbac.Rule {
	return simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser)
}
