package process

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/go-clir"
)

const Type = "process"

type Actions interface {
	Install(ctx context.Context) error
	Upgrade(ctx context.Context) error
	Quit(ctx context.Context) error
}

type Component struct {
	Actions Actions
}

var _ component.Component = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)

type installCommand struct{}
type upgradeCommand struct{}
type quitCommand struct{}

func New(actions Actions) *Component {
	return &Component{Actions: actions}
}

func (c *Component) Type() string {
	return Type
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		processCommandDefinition(
			"process.install",
			"Install ctgbot binaries from source",
			buildInstallCommand,
			commandengine.SourceMessage,
			[]string{"process install", "install"},
		),
		processCommandDefinition(
			"process.install.cli",
			"Install ctgbot binaries from source",
			buildInstallCommand,
			commandengine.SourceCLI,
			[]string{"process install"},
		),
		processCommandDefinition(
			"process.upgrade",
			"Upgrade ctgbot",
			buildUpgradeCommand,
			commandengine.SourceMessage,
			[]string{"process upgrade", "upgrade"},
		),
		processCommandDefinition(
			"process.upgrade.cli",
			"Upgrade ctgbot",
			buildUpgradeCommand,
			commandengine.SourceCLI,
			[]string{"process upgrade"},
		),
		processCommandDefinition(
			"process.quit",
			"Restart ctgbot",
			buildQuitCommand,
			commandengine.SourceMessage,
			[]string{"process quit", "quit"},
		),
		processCommandDefinition(
			"process.quit.cli",
			"Restart ctgbot",
			buildQuitCommand,
			commandengine.SourceCLI,
			[]string{"process quit"},
		),
	}
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := commandengine.Register[installCommand](
		registry,
		func(ctx context.Context, req commandengine.Request, cmd installCommand) (commandengine.Result, error) {
			_, _, _ = req, cmd, c
			if err := c.install(ctx); err != nil {
				return commandengine.Result{}, err
			}
			return commandengine.Result{Text: "install completed\ntype /quit to restart"}, nil
		},
	); err != nil {
		return err
	}
	if err := commandengine.Register[upgradeCommand](
		registry,
		func(ctx context.Context, req commandengine.Request, cmd upgradeCommand) (commandengine.Result, error) {
			_, _, _ = req, cmd, c
			if err := c.upgrade(ctx); err != nil {
				return commandengine.Result{}, err
			}
			return commandengine.Result{Text: "upgrade completed\ntype /quit to restart"}, nil
		},
	); err != nil {
		return err
	}
	return commandengine.Register[quitCommand](
		registry,
		func(ctx context.Context, req commandengine.Request, cmd quitCommand) (commandengine.Result, error) {
			_, _, _ = req, cmd, c
			if err := c.quit(ctx); err != nil {
				return commandengine.Result{}, err
			}
			return commandengine.Result{Text: "quit requested"}, nil
		},
	)
}

func (c *Component) install(ctx context.Context) error {
	if c == nil || c.Actions == nil {
		return fmt.Errorf("missing process actions")
	}
	return c.Actions.Install(ctx)
}

func (c *Component) upgrade(ctx context.Context) error {
	if c == nil || c.Actions == nil {
		return fmt.Errorf("missing process actions")
	}
	return c.Actions.Upgrade(ctx)
}

func (c *Component) quit(ctx context.Context) error {
	if c == nil || c.Actions == nil {
		return fmt.Errorf("missing process actions")
	}
	return c.Actions.Quit(ctx)
}

func processCommandDefinition(id string, help string, build commandengine.BuildFunc, source commandengine.Source, patterns []string) commandengine.Definition {
	routes := make([]commandengine.Route, 0, len(patterns))
	for _, pattern := range patterns {
		routes = append(routes, commandengine.Route{
			Pattern: pattern,
			Help:    help,
			Build:   build,
		})
	}
	return commandengine.Definition{
		ID:      id,
		Sources: []commandengine.Source{source},
		Policy:  simplerbac.Any(simplerbac.RoleRoot),
		Routes:  routes,
	}
}

func buildInstallCommand(req *clir.Request) (any, error) {
	_ = req
	return installCommand{}, nil
}

func buildUpgradeCommand(req *clir.Request) (any, error) {
	_ = req
	return upgradeCommand{}, nil
}

func buildQuitCommand(req *clir.Request) (any, error) {
	_ = req
	return quitCommand{}, nil
}
