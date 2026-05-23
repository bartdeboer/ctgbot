package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/ctgbot/internal/system"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

type workspaceCLISurface struct {
	store *clistate.Store
}

type workspaceSetCommand struct {
	Name string
	Path string
}

type workspaceListCommand struct{}

var _ component.Component = (*workspaceCLISurface)(nil)
var _ component.CommandSurface = (*workspaceCLISurface)(nil)

func newWorkspaceCLISurface(store *clistate.Store) *workspaceCLISurface {
	return &workspaceCLISurface{store: store}
}

func (s *workspaceCLISurface) Type() string { return "workspace" }

func (s *workspaceCLISurface) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		{
			Pattern: "workspace set <workspace>",
			Help:    "Configure a workspace",
			Build:   buildWorkspaceSetCommand,
			Sources: []commandengine.Source{commandengine.SourceCLI},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
		},
		{
			Pattern: "workspace list",
			Help:    "List configured workspaces",
			Build: func(req *clir.Request) (any, error) {
				_ = req
				return workspaceListCommand{}, nil
			},
			Sources: []commandengine.Source{commandengine.SourceCLI},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
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
	_, _ = ctx, req
	rootDir, err := filepath.Abs(".")
	if err != nil {
		return commandengine.Result{}, err
	}
	workspace, err := system.SaveWorkspace(rootDir, s.store, cmd.Name, cmd.Path)
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
	_, _, _ = ctx, req, cmd
	rootDir, err := filepath.Abs(".")
	if err != nil {
		return commandengine.Result{}, err
	}
	workspaces, err := system.LoadWorkspaces(rootDir, s.store)
	if err != nil {
		return commandengine.Result{}, err
	}
	configured := system.ConfiguredWorkspaces(s.store)
	names := make([]string, 0, len(workspaces))
	for name := range workspaces {
		names = append(names, name)
	}
	slices.Sort(names)
	if len(names) == 0 {
		return commandengine.Result{Text: "no workspaces"}, nil
	}
	lines := make([]string, 0, len(names))
	for _, name := range names {
		workspace := workspaces[name]
		_, ok := configured[name]
		lines = append(lines, fmt.Sprintf("%s\tpath=%s\tconfigured=%t", workspace.Name, workspace.Path, ok))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}
