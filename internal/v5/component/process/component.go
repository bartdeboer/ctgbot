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
	Quit(ctx context.Context) error
}

type Component struct {
	Actions Actions
}

var _ component.Component = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)

type quitCommand struct{}

func New(actions Actions) *Component {
	return &Component{Actions: actions}
}

func (c *Component) Type() string {
	return Type
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		{
			ID:      "process.quit.message",
			Sources: []commandengine.Source{commandengine.SourceMessage},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
			Routes: []commandengine.Route{
				{
					Pattern: "process quit",
					Help:    "Restart ctgbot",
					Build:   buildQuitCommand,
				},
				{
					Pattern: "quit",
					Help:    "Restart ctgbot",
					Build:   buildQuitCommand,
				},
			},
		},
		{
			ID:      "process.quit.agent",
			Sources: []commandengine.Source{commandengine.SourceHostbridge, commandengine.SourceCLI},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
			Routes: []commandengine.Route{{
				Pattern: "process quit",
				Help:    "Restart ctgbot",
				Build:   buildQuitCommand,
			}},
		},
	}
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
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

func (c *Component) quit(ctx context.Context) error {
	if c == nil || c.Actions == nil {
		return fmt.Errorf("missing process actions")
	}
	return c.Actions.Quit(ctx)
}

func buildQuitCommand(req *clir.Request) (any, error) {
	_ = req
	return quitCommand{}, nil
}
