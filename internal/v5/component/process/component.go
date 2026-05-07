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
	definitions := make([]commandengine.Definition, 0, 6)
	definitions = append(definitions, processCommandDefinitions(
		"install",
		"process install",
		"Install ctgbot binaries from source",
		buildInstallCommand,
		[]commandengine.Route{{Pattern: "install", Absolute: true}},
	)...)
	definitions = append(definitions, processCommandDefinitions(
		"upgrade",
		"process upgrade",
		"Upgrade ctgbot",
		buildUpgradeCommand,
		[]commandengine.Route{{Pattern: "upgrade", Absolute: true}},
	)...)
	definitions = append(definitions, processCommandDefinitions(
		"quit",
		"process quit",
		"Restart ctgbot",
		buildQuitCommand,
		[]commandengine.Route{{Pattern: "quit", Absolute: true}},
	)...)
	return definitions
}

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := registerProcessPattern[installCommand](registry, []string{"install", "process install"}, func(ctx context.Context) (commandengine.Result, error) {
		if err := c.install(ctx); err != nil {
			return commandengine.Result{}, err
		}
		return commandengine.Result{Text: "install completed\ntype /quit to restart"}, nil
	}); err != nil {
		return err
	}
	if err := registerProcessPattern[upgradeCommand](registry, []string{"upgrade", "process upgrade"}, func(ctx context.Context) (commandengine.Result, error) {
		if err := c.upgrade(ctx); err != nil {
			return commandengine.Result{}, err
		}
		return commandengine.Result{Text: "upgrade completed\ntype /quit to restart"}, nil
	}); err != nil {
		return err
	}
	return registerProcessPattern[quitCommand](registry, []string{"quit", "process quit"}, func(ctx context.Context) (commandengine.Result, error) {
		if err := c.quit(ctx); err != nil {
			return commandengine.Result{}, err
		}
		return commandengine.Result{Text: "quit requested"}, nil
	})
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

func processCommandDefinitions(localPattern string, cliPattern string, help string, build commandengine.BuildFunc, aliases []commandengine.Route) []commandengine.Definition {
	return []commandengine.Definition{
		processCommandDefinition(localPattern, help, build, []commandengine.Source{commandengine.SourceMessage}, aliases),
		processCommandDefinition(cliPattern, help, build, []commandengine.Source{commandengine.SourceCLI}, nil),
	}
}

func processCommandDefinition(pattern string, help string, build commandengine.BuildFunc, sources []commandengine.Source, aliases []commandengine.Route) commandengine.Definition {
	return commandengine.Definition{
		Pattern: pattern,
		Help:    help,
		Build:   build,
		Sources: sources,
		Policy:  simplerbac.Any(simplerbac.RoleRoot),
		Aliases: aliases,
	}
}

func registerProcessPattern[T any](registry *commandengine.Registry, patterns []string, handler func(ctx context.Context) (commandengine.Result, error)) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if handler == nil {
		return fmt.Errorf("missing process command handler")
	}
	for _, pattern := range patterns {
		pattern := commandengine.NormalizePattern(pattern)
		if pattern == "" {
			return fmt.Errorf("missing process command pattern")
		}
		if err := commandengine.RegisterPattern[T](registry, pattern, func(ctx context.Context, req commandengine.Request, cmd T) (commandengine.Result, error) {
			_, _ = req, cmd
			return handler(ctx)
		}); err != nil {
			return err
		}
	}
	return nil
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
