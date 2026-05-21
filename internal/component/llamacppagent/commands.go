package llamacppagent

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

var _ component.CommandSurface = (*Component)(nil)
var _ component.LocalCommandSurface = (*Component)(nil)

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		llamacppAgentCommand("container refresh", RefreshContainer{}, "Delete and recreate the llama.cpp agent runtime on next turn"),
	}
}

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	return commandengine.RegisterPattern[RefreshContainer](registry, "container refresh", func(ctx context.Context, req commandengine.Request, _ RefreshContainer) (commandengine.Result, error) {
		return c.refresh(ctx, req)
	})
}

func (c *Component) refresh(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, workspacePath, err := agentcommon.ThreadWorkspace(ctx, c.storage, c.resolveWorkspace, req, Type)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := c.runtime.Refresh(ctx, workspacePath, thread.ID); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "llama.cpp agent runtime refreshed"}, nil
}

func llamacppAgentCommand(pattern string, command any, help string) commandengine.Definition {
	return commandengine.Definition{
		Pattern: pattern,
		Help:    help,
		Build:   func(req *clir.Request) (any, error) { _ = req; return command, nil },
		Sources: []commandengine.Source{
			commandengine.SourceMessage,
			commandengine.SourceHostbridge,
		},
		Policy:                simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser),
		InstructionVisibility: commandengine.InstructionImportant,
	}
}
