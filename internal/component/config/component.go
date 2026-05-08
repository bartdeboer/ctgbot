package config

import (
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/configengine"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
	configschema "github.com/bartdeboer/ctgbot/internal/schema/config"
	"github.com/bartdeboer/ctgbot/internal/schema/routers"
)

const Type = "config"

type Component struct {
	manager *configengine.Manager
}

var _ component.Component = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)

func New(cfg *appstate.Config) (*Component, error) {
	if cfg == nil {
		return nil, nil
	}
	registry, err := configschema.Registry(cfg)
	if err != nil {
		return nil, err
	}
	return &Component{manager: configengine.New(registry)}, nil
}

func (c *Component) Type() string {
	return Type
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	all := schemacommands.ConfigCommands()
	out := make([]commandengine.Definition, 0, 3)
	for _, definition := range all {
		switch definition.CanonicalPattern() {
		case "config list", "config get <key>", "config set <key> <value>":
			out = append(out, definition)
		}
	}
	return out
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if c == nil || c.manager == nil {
		return fmt.Errorf("missing config manager")
	}
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	return routers.RegisterConfigHandlers(registry, c.manager)
}
