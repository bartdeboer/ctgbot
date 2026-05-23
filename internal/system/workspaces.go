package system

import (
	workspacepkg "github.com/bartdeboer/ctgbot/internal/workspace"
	"github.com/bartdeboer/go-clistate"
)

type WorkspaceSettings = workspacepkg.Settings
type WorkspaceHostbridgeSettings = workspacepkg.HostbridgeSettings
type Workspace = workspacepkg.Workspace

const workspaceConfigKey = workspacepkg.ConfigKey

type WorkspaceManagerProvider interface {
	WorkspaceManager() workspacepkg.Manager
}

func ConfiguredWorkspaces(store *clistate.Store) map[string]WorkspaceSettings {
	return workspacepkg.Configured(store)
}

func LoadWorkspaces(rootDir string, store *clistate.Store) (map[string]Workspace, error) {
	return workspacepkg.Load(rootDir, store)
}

func SaveWorkspace(rootDir string, store *clistate.Store, name string, path string) (Workspace, error) {
	return workspacepkg.Save(rootDir, store, name, path)
}

func (s *System) WorkspaceManager() workspacepkg.Manager {
	if s == nil {
		return workspacepkg.Manager{}
	}
	return workspacepkg.Manager{RootDir: s.RootDir, Store: s.Store}
}
