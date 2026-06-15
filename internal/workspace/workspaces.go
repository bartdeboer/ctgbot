package workspace

import (
	"fmt"
	"path/filepath"
	"strings"

	hostbridgepolicy "github.com/bartdeboer/ctgbot/internal/hostbridgepolicy"
	"github.com/bartdeboer/go-clistate"
)

const ConfigKey = "workspaces"

type Settings struct {
	Path       string             `json:"path"`
	Hostbridge HostbridgeSettings `json:"hostbridge"`
}

type HostbridgeSettings struct {
	Aliases map[string]hostbridgepolicy.Alias `json:"allowed_commands"`
}

type Workspace struct {
	Name              string
	Path              string
	HostbridgeAliases map[string]hostbridgepolicy.Alias
}

type Info struct {
	Workspace  Workspace
	Configured bool
}

type Manager struct {
	RootDir string
	Store   *clistate.Store
}

func (m Manager) List() ([]Info, error) {
	workspaces, err := Load(m.RootDir, m.Store)
	if err != nil {
		return nil, err
	}
	configured := Configured(m.Store)
	names := make([]string, 0, len(workspaces))
	for name := range workspaces {
		names = append(names, name)
	}
	slicesSort(names)
	out := make([]Info, 0, len(names))
	for _, name := range names {
		_, ok := configured[name]
		out = append(out, Info{Workspace: workspaces[name], Configured: ok})
	}
	return out, nil
}

func (m Manager) Save(name string, path string) (Workspace, error) {
	return Save(m.RootDir, m.Store, name, path)
}

func Configured(store *clistate.Store) map[string]Settings {
	if store == nil {
		return map[string]Settings{}
	}
	var out map[string]Settings
	store.GetStruct(ConfigKey, &out)
	if out == nil {
		return map[string]Settings{}
	}
	return out
}

func Load(rootDir string, store *clistate.Store) (map[string]Workspace, error) {
	rootDir = strings.TrimSpace(rootDir)
	if rootDir == "" {
		return nil, fmt.Errorf("missing root dir")
	}
	configured := Configured(store)
	workspaces := map[string]Workspace{}
	for name, settings := range configured {
		workspace, err := resolve(rootDir, name, settings)
		if err != nil {
			return nil, err
		}
		workspaces[name] = workspace
	}
	return workspaces, nil
}

func Save(rootDir string, store *clistate.Store, name string, path string) (Workspace, error) {
	if store == nil {
		return Workspace{}, fmt.Errorf("missing workspace store")
	}
	name = strings.TrimSpace(name)
	if err := validateName(name); err != nil {
		return Workspace{}, err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return Workspace{}, fmt.Errorf("missing workspace path")
	}

	configured := Configured(store)
	settings := configured[name]
	settings.Path = path
	configured[name] = settings
	if err := store.PersistStruct(ConfigKey, configured); err != nil {
		return Workspace{}, err
	}
	return resolve(rootDir, name, configured[name])
}

func resolve(rootDir string, name string, settings Settings) (Workspace, error) {
	if err := validateName(name); err != nil {
		return Workspace{}, err
	}
	path, err := resolvePath(rootDir, strings.TrimSpace(settings.Path))
	if err != nil {
		return Workspace{}, err
	}
	return Workspace{
		Name:              name,
		Path:              path,
		HostbridgeAliases: normalizeAliases(settings.Hostbridge.Aliases),
	}, nil
}

func resolvePath(rootDir string, path string) (string, error) {
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

func validateName(name string) error {
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

func normalizeAliases(raw map[string]hostbridgepolicy.Alias) map[string]hostbridgepolicy.Alias {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]hostbridgepolicy.Alias, len(raw))
	for name, spec := range raw {
		name = strings.TrimSpace(name)
		spec.Name = strings.TrimSpace(spec.Name)
		spec.Dir = strings.TrimSpace(spec.Dir)
		spec.Delay = strings.TrimSpace(spec.Delay)
		if name == "" || spec.Name == "" {
			continue
		}
		if len(spec.Args) == 0 {
			spec.Args = nil
		}
		if len(spec.Env) == 0 {
			spec.Env = nil
		}
		out[name] = spec
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func slicesSort(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
