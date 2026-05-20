package process

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/buildassets"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

const Type = "process"

type Actions interface {
	Install(ctx context.Context) error
	Upgrade(ctx context.Context, all bool) error
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
type upgradeAllCommand struct{}
type quitCommand struct{}
type versionCommand struct{}

func New(actions Actions) *Component {
	return &Component{Actions: actions}
}

func (c *Component) Type() string {
	return Type
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	definitions := make([]commandengine.Definition, 0, 8)
	definitions = append(definitions, processCommandDefinitions(
		"install",
		"Install ctgbot binaries from source",
		buildInstallCommand,
		[]commandengine.Route{{Pattern: "install", Absolute: true}},
		simplerbac.Any(simplerbac.RoleRoot),
	)...)
	definitions = append(definitions, processCommandDefinitions(
		"upgrade",
		"Upgrade ctgbot",
		buildUpgradeCommand,
		[]commandengine.Route{{Pattern: "upgrade", Absolute: true}},
		simplerbac.Any(simplerbac.RoleRoot),
	)...)
	definitions = append(definitions, processCommandDefinitions(
		"upgrade all",
		"Upgrade ctgbot and rebuild runtime images without cache",
		buildUpgradeAllCommand,
		[]commandengine.Route{{Pattern: "upgrade all", Absolute: true}},
		simplerbac.Any(simplerbac.RoleRoot),
	)...)
	definitions = append(definitions, processCommandDefinitions(
		"quit",
		"Stop ctgbot",
		buildQuitCommand,
		[]commandengine.Route{{Pattern: "quit", Absolute: true}},
		simplerbac.Any(simplerbac.RoleRoot),
	)...)
	definitions = append(definitions, processCommandDefinitions(
		"version",
		"Show ctgbot version",
		buildVersionCommand,
		[]commandengine.Route{{Pattern: "version", Absolute: true}},
		simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser),
	)...)
	return definitions
}

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := registerProcessPattern[installCommand](registry, "install", func(ctx context.Context) (commandengine.Result, error) {
		if err := c.install(ctx); err != nil {
			return commandengine.Result{}, err
		}
		return commandengine.Result{Text: "install completed\ntype /quit to restart"}, nil
	}); err != nil {
		return err
	}
	if err := registerProcessPattern[upgradeCommand](registry, "upgrade", func(ctx context.Context) (commandengine.Result, error) {
		if err := c.upgrade(ctx, false); err != nil {
			return commandengine.Result{}, err
		}
		return commandengine.Result{Text: "upgrade completed\ntype /quit to restart"}, nil
	}); err != nil {
		return err
	}
	if err := registerProcessPattern[upgradeAllCommand](registry, "upgrade all", func(ctx context.Context) (commandengine.Result, error) {
		if err := c.upgrade(ctx, true); err != nil {
			return commandengine.Result{}, err
		}
		return commandengine.Result{Text: "upgrade all completed\ntype /quit to restart"}, nil
	}); err != nil {
		return err
	}
	if err := registerProcessPattern[quitCommand](registry, "quit", func(ctx context.Context) (commandengine.Result, error) {
		if err := c.quit(ctx); err != nil {
			return commandengine.Result{}, err
		}
		return commandengine.Result{Text: "quit requested"}, nil
	}); err != nil {
		return err
	}
	return registerProcessPattern[versionCommand](registry, "version", func(ctx context.Context) (commandengine.Result, error) {
		_ = ctx
		return commandengine.Result{Text: buildassets.Version()}, nil
	})
}

func (c *Component) install(ctx context.Context) error {
	if c == nil || c.Actions == nil {
		return fmt.Errorf("missing process actions")
	}
	return c.Actions.Install(ctx)
}

func (c *Component) upgrade(ctx context.Context, all bool) error {
	if c == nil || c.Actions == nil {
		return fmt.Errorf("missing process actions")
	}
	return c.Actions.Upgrade(ctx, all)
}

func (c *Component) quit(ctx context.Context) error {
	if c == nil || c.Actions == nil {
		return fmt.Errorf("missing process actions")
	}
	return c.Actions.Quit(ctx)
}

func processCommandDefinitions(localPattern string, help string, build commandengine.BuildFunc, aliases []commandengine.Route, policy simplerbac.Rule) []commandengine.Definition {
	return []commandengine.Definition{
		processCommandDefinition(localPattern, help, build, []commandengine.Source{commandengine.SourceMessage}, aliases, policy),
		processCommandDefinition(localPattern, help, build, []commandengine.Source{commandengine.SourceCLI}, nil, policy),
	}
}

func processCommandDefinition(pattern string, help string, build commandengine.BuildFunc, sources []commandengine.Source, aliases []commandengine.Route, policy simplerbac.Rule) commandengine.Definition {
	return commandengine.Definition{
		Pattern: pattern,
		Help:    help,
		Build:   build,
		Sources: sources,
		Policy:  policy,
		Aliases: aliases,
	}
}

func registerProcessPattern[T any](registry *commandengine.Registry, pattern string, handler func(ctx context.Context) (commandengine.Result, error)) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if handler == nil {
		return fmt.Errorf("missing process command handler")
	}
	pattern = commandengine.NormalizePattern(pattern)
	if pattern == "" {
		return fmt.Errorf("missing process command pattern")
	}
	return commandengine.RegisterPattern[T](registry, pattern, func(ctx context.Context, req commandengine.Request, cmd T) (commandengine.Result, error) {
		_, _ = req, cmd
		return handler(ctx)
	})
}

func buildInstallCommand(req *clir.Request) (any, error) {
	_ = req
	return installCommand{}, nil
}

func buildUpgradeCommand(req *clir.Request) (any, error) {
	_ = req
	return upgradeCommand{}, nil
}

func buildUpgradeAllCommand(req *clir.Request) (any, error) {
	_ = req
	return upgradeAllCommand{}, nil
}

func buildQuitCommand(req *clir.Request) (any, error) {
	_ = req
	return quitCommand{}, nil
}

func buildVersionCommand(req *clir.Request) (any, error) {
	_ = req
	return versionCommand{}, nil
}
