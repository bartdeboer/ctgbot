package llamacpp

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
	"github.com/bartdeboer/go-clir"
)

var _ component.CommandSurface = (*Component)(nil)

type startCommand struct{}
type stopCommand struct{}
type statusCommand struct{}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		llamacppCommand("llamacpp.start", startCommand{}, "Start the llama.cpp model service", []string{"llamacpp start"}),
		llamacppCommand("llamacpp.stop", stopCommand{}, "Stop the llama.cpp model service", []string{"llamacpp stop"}),
		llamacppCommand("llamacpp.status", statusCommand{}, "Show llama.cpp model service status", []string{"llamacpp status"}),
	}
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := commandengine.Register[startCommand](registry, func(ctx context.Context, req commandengine.Request, cmd startCommand) (commandengine.Result, error) {
		_, _ = req, cmd
		return c.start(ctx)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[stopCommand](registry, func(ctx context.Context, req commandengine.Request, cmd stopCommand) (commandengine.Result, error) {
		_, _ = req, cmd
		return c.stop(ctx)
	}); err != nil {
		return err
	}
	return commandengine.Register[statusCommand](registry, func(ctx context.Context, req commandengine.Request, cmd statusCommand) (commandengine.Result, error) {
		_, _ = req, cmd
		return c.status(ctx)
	})
}

func (c *Component) start(ctx context.Context) (commandengine.Result, error) {
	if c == nil || c.runtime == nil {
		return commandengine.Result{}, fmt.Errorf("missing llamacpp backend runtime")
	}
	status, err := c.runtime.Start(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: c.formatStatus("llama.cpp started", status)}, nil
}

func (c *Component) stop(ctx context.Context) (commandengine.Result, error) {
	if c == nil || c.runtime == nil {
		return commandengine.Result{}, fmt.Errorf("missing llamacpp backend runtime")
	}
	if err := c.runtime.Stop(ctx); err != nil {
		return commandengine.Result{}, err
	}
	status, err := c.runtime.Status(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: c.formatStatus("llama.cpp stopped", status)}, nil
}

func (c *Component) status(ctx context.Context) (commandengine.Result, error) {
	if c == nil || c.runtime == nil {
		return commandengine.Result{}, fmt.Errorf("missing llamacpp backend runtime")
	}
	status, err := c.runtime.Status(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: c.formatStatus("llama.cpp status", status)}, nil
}

func (c *Component) formatStatus(title string, status v5runtime.Status) string {
	lines := []string{strings.TrimSpace(title)}
	lines = append(lines,
		"state: "+status.State,
		"container: "+status.Name,
	)
	lines = append(lines,
		"model: "+c.componentConfig.ModelPath,
		fmt.Sprintf("port: %d", c.componentConfig.HostPort),
	)
	if strings.TrimSpace(c.componentConfig.MMProjPath) != "" {
		lines = append(lines, "mmproj: "+c.componentConfig.MMProjPath)
	}
	return strings.Join(lines, "\n")
}

func llamacppCommand(id string, command any, help string, patterns []string) commandengine.Definition {
	routes := make([]commandengine.Route, 0, len(patterns))
	for _, pattern := range patterns {
		command := command
		routes = append(routes, commandengine.Route{
			Pattern: pattern,
			Help:    help,
			Build: func(req *clir.Request) (any, error) {
				_ = req
				return command, nil
			},
		})
	}
	return commandengine.Definition{
		ID: id,
		Sources: []commandengine.Source{
			commandengine.SourceMessage,
			commandengine.SourceHostbridge,
		},
		Policy: simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser),
		Routes: routes,
	}
}
