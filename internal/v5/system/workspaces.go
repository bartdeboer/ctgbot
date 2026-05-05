package system

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/go-clistate"
)

const workspaceConfigKey = "workspaces"

type WorkspaceSettings struct {
	Path string `json:"path"`
}

type Workspace struct {
	Name string
	Path string
}

func ConfiguredWorkspaces(store *clistate.Store) map[string]WorkspaceSettings {
	if store == nil {
		return map[string]WorkspaceSettings{}
	}
	var out map[string]WorkspaceSettings
	store.GetStruct(workspaceConfigKey, &out)
	if out == nil {
		return map[string]WorkspaceSettings{}
	}
	return out
}

func LoadWorkspaces(rootDir string, store *clistate.Store) (map[string]Workspace, error) {
	rootDir = strings.TrimSpace(rootDir)
	if rootDir == "" {
		return nil, fmt.Errorf("missing root dir")
	}
	configured := ConfiguredWorkspaces(store)
	workspaces := map[string]Workspace{}
	for name, settings := range configured {
		workspace, err := resolveWorkspace(rootDir, name, settings)
		if err != nil {
			return nil, err
		}
		workspaces[name] = workspace
	}
	return workspaces, nil
}

func SaveWorkspace(rootDir string, store *clistate.Store, name string, path string) (Workspace, error) {
	if store == nil {
		return Workspace{}, fmt.Errorf("missing workspace store")
	}
	name = strings.TrimSpace(name)
	if err := validateWorkspaceName(name); err != nil {
		return Workspace{}, err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return Workspace{}, fmt.Errorf("missing workspace path")
	}

	configured := ConfiguredWorkspaces(store)
	configured[name] = WorkspaceSettings{Path: path}
	if err := store.PersistStruct(workspaceConfigKey, configured); err != nil {
		return Workspace{}, err
	}
	return resolveWorkspace(rootDir, name, configured[name])
}

func resolveWorkspace(rootDir string, name string, settings WorkspaceSettings) (Workspace, error) {
	if err := validateWorkspaceName(name); err != nil {
		return Workspace{}, err
	}
	path, err := resolveWorkspacePath(rootDir, strings.TrimSpace(settings.Path))
	if err != nil {
		return Workspace{}, err
	}
	return Workspace{Name: name, Path: path}, nil
}

func resolveWorkspacePath(rootDir string, path string) (string, error) {
	rootDir = strings.TrimSpace(rootDir)
	if rootDir == "" {
		return "", fmt.Errorf("missing root dir")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("missing workspace path")
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	return filepath.Clean(filepath.Join(rootDir, path)), nil
}

func validateWorkspaceName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("missing workspace name")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid workspace name: %q", name)
	}
	if strings.ContainsAny(name, `/\\`) {
		return fmt.Errorf("invalid workspace name: %q", name)
	}
	if filepath.Clean(name) != name {
		return fmt.Errorf("invalid workspace name: %q", name)
	}
	return nil
}
