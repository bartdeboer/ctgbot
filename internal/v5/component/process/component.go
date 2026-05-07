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
var _ component.LocalCommandSurface = (*Component)(nil)

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
			"install",
			"Install ctgbot binaries from source",
			buildInstallCommand,
			commandengine.SourceMessage,
			[]commandengine.Route{
				{Pattern: "install", Absolute: true},
			},
		),
		processCommandDefinition(
			"process install",
			"Install ctgbot binaries from source",
			buildInstallCommand,
			commandengine.SourceCLI,
			nil,
		),
		processCommandDefinition(
			"upgrade",
			"Upgrade ctgbot",
			buildUpgradeCommand,
			commandengine.SourceMessage,
			[]commandengine.Route{
				{Pattern: "upgrade", Absolute: true},
			},
		),
		processCommandDefinition(
			"process upgrade",
			"Upgrade ctgbot",
			buildUpgradeCommand,
			commandengine.SourceCLI,
			nil,
		),
		processCommandDefinition(
			"quit",
			"Restart ctgbot",
			buildQuitCommand,
			commandengine.SourceMessage,
			[]commandengine.Route{
				{Pattern: "quit", Absolute: true},
			},
		),
		processCommandDefinition(
			"process quit",
			"Restart ctgbot",
			buildQuitCommand,
			commandengine.SourceCLI,
			nil,
		),
	}
}

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := commandengine.RegisterDefinition[installCommand](
		registry,
		"install",
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
	if err := commandengine.RegisterDefinition[upgradeCommand](
		registry,
		"upgrade",
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
	if err := commandengine.RegisterDefinition[quitCommand](
		registry,
		"quit",
		func(ctx context.Context, req commandengine.Request, cmd quitCommand) (commandengine.Result, error) {
			_, _, _ = req, cmd, c
			if err := c.quit(ctx); err != nil {
				return commandengine.Result{}, err
			}
			return commandengine.Result{Text: "quit requested"}, nil
		},
	); err != nil {
		return err
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

func processCommandDefinition(pattern string, help string, build commandengine.BuildFunc, source commandengine.Source, aliases []commandengine.Route) commandengine.Definition {
	return commandengine.Definition{
		Pattern: pattern,
		Help:    help,
		Build:   build,
		Sources: []commandengine.Source{source},
		Policy:  simplerbac.Any(simplerbac.RoleRoot),
		Aliases: aliases,
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
