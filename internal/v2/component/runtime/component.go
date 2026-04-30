package runtime

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/v2/component"
	v2commands "github.com/bartdeboer/ctgbot/internal/v2/schema/commands"
	v2routers "github.com/bartdeboer/ctgbot/internal/v2/schema/routers"
)

const ComponentType = "runtime"

type Actions interface {
	Install(ctx context.Context) error
	Quit(ctx context.Context) error
}

type Component struct {
	Actions Actions
}

var _ component.Component = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)
var _ v2routers.ProcessHandlers = (*Component)(nil)

func New(actions Actions) *Component {
	return &Component{Actions: actions}
}

func (c *Component) Type() string {
	return ComponentType
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return v2commands.ProcessCommands()
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	return v2routers.RegisterProcessHandlers(registry, c)
}

func (c *Component) Install(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	actions, err := c.actions()
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := actions.Install(ctx); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "install completed\ntype /quit to restart"}, nil
}

func (c *Component) Quit(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	actions, err := c.actions()
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := actions.Quit(ctx); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "quit requested"}, nil
}

func (c *Component) actions() (Actions, error) {
	if c == nil || c.Actions == nil {
		return nil, fmt.Errorf("missing runtime actions")
	}
	return c.Actions, nil
}
