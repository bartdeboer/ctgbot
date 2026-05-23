package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type workspaceCLISurface struct {
	service *service
}

type workspaceSetCommand struct {
	Name string
	Path string
}

type workspaceListCommand struct{}

var _ component.CommandSurface = (*workspaceCLISurface)(nil)

func newWorkspaceCLISurface(service *service) *workspaceCLISurface {
	return &workspaceCLISurface{service: service}
}

func (s *workspaceCLISurface) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		{
			Pattern:               "workspace set <workspace>",
			Help:                  "Configure a workspace",
			Build:                 buildWorkspaceSetCommand,
			Sources:               []commandengine.Source{commandengine.SourceCLI},
			Policy:                simplerbac.Any(simplerbac.RoleRoot),
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "workspace list",
			Help:    "List configured workspaces",
			Build: func(req *clir.Request) (any, error) {
				_ = req
				return workspaceListCommand{}, nil
			},
			Sources:               []commandengine.Source{commandengine.SourceCLI},
			Policy:                simplerbac.Any(simplerbac.RoleRoot),
			InstructionVisibility: commandengine.InstructionImportant,
		},
	}
}

func (s *workspaceCLISurface) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := commandengine.Register[workspaceSetCommand](registry, s.handleWorkspaceSet); err != nil {
		return err
	}
	return commandengine.Register[workspaceListCommand](registry, s.handleWorkspaceList)
}

func buildWorkspaceSetCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("workspace set", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	path := fs.String("path", "", "Host workspace path")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Params["workspace"])
	if name == "" {
		return nil, fmt.Errorf("missing workspace name")
	}
	if strings.TrimSpace(*path) == "" {
		return nil, fmt.Errorf("missing workspace path")
	}
	return workspaceSetCommand{Name: name, Path: strings.TrimSpace(*path)}, nil
}

func (s *workspaceCLISurface) handleWorkspaceSet(ctx context.Context, req commandengine.Request, cmd workspaceSetCommand) (commandengine.Result, error) {
	_ = req
	if s == nil || s.service == nil {
		return commandengine.Result{}, fmt.Errorf("missing app service")
	}
	workspace, err := s.service.SaveWorkspace(ctx, cmd.Name, cmd.Path)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: strings.Join([]string{
		"workspace saved",
		"name: " + workspace.Name,
		"path: " + workspace.Path,
	}, "\n")}, nil
}

func (s *workspaceCLISurface) handleWorkspaceList(ctx context.Context, req commandengine.Request, cmd workspaceListCommand) (commandengine.Result, error) {
	_, _ = req, cmd
	if s == nil || s.service == nil {
		return commandengine.Result{}, fmt.Errorf("missing app service")
	}
	workspaces, err := s.service.ListWorkspaces(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	if len(workspaces) == 0 {
		return commandengine.Result{Text: "no workspaces"}, nil
	}
	lines := make([]string, 0, len(workspaces))
	for _, info := range workspaces {
		workspace := info.Workspace
		lines = append(lines, fmt.Sprintf("%s\tpath=%s\tconfigured=%t", workspace.Name, workspace.Path, info.Configured))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}
