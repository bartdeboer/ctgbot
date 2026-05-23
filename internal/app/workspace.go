package app

import (
	"context"
	"fmt"

	workspacepkg "github.com/bartdeboer/ctgbot/internal/workspace"
)

type WorkspaceManagerProvider interface {
	WorkspaceManager() workspacepkg.Manager
}

type WorkspaceAdminService interface {
	SaveWorkspace(ctx context.Context, name string, path string) (workspacepkg.Workspace, error)
	ListWorkspaces(ctx context.Context) ([]workspacepkg.Info, error)
}

func (s *service) SaveWorkspace(ctx context.Context, name string, path string) (workspacepkg.Workspace, error) {
	_ = ctx
	if s == nil || s.Workspaces.Store == nil {
		return workspacepkg.Workspace{}, fmt.Errorf("missing workspace manager")
	}
	return s.Workspaces.Save(name, path)
}

func (s *service) ListWorkspaces(ctx context.Context) ([]workspacepkg.Info, error) {
	_ = ctx
	if s == nil || s.Workspaces.Store == nil {
		return nil, fmt.Errorf("missing workspace manager")
	}
	return s.Workspaces.List()
}
