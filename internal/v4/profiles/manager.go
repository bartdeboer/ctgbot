package profiles

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/go-clistate"
)

const configKey = "v4.profiles"

type Settings struct {
	Runtime  string `json:"runtime"`
	HomePath string `json:"home_path"`
}

type Profile struct {
	Name    string
	Runtime string
	Root    string
}

type Manager struct {
	RootDir  string
	StateDir string
	Store    *clistate.Store
}

func New(rootDir string, store *clistate.Store) *Manager {
	return &Manager{
		RootDir:  strings.TrimSpace(rootDir),
		StateDir: ".ctgbot",
		Store:    store,
	}
}

func (m *Manager) Resolve(name string) (Profile, error) {
	if m == nil {
		return Profile{}, fmt.Errorf("missing profile manager")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "default"
	}
	if err := validateName("profile", name); err != nil {
		return Profile{}, err
	}
	settings := m.settings()[name]
	runtimeDriver := strings.TrimSpace(settings.Runtime)
	if runtimeDriver == "" {
		runtimeDriver = "docker"
	}
	root, err := m.profileRoot(name, settings)
	if err != nil {
		return Profile{}, err
	}
	return Profile{
		Name:    name,
		Runtime: runtimeDriver,
		Root:    root,
	}, nil
}

func (m *Manager) Set(name string, settings Settings) error {
	if m == nil || m.Store == nil {
		return fmt.Errorf("missing profile store")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("missing profile name")
	}
	if err := validateName("profile", name); err != nil {
		return err
	}
	settings.Runtime = strings.TrimSpace(settings.Runtime)
	settings.HomePath = strings.TrimSpace(settings.HomePath)
	all := m.settings()
	all[name] = settings
	return m.Store.PersistStruct(configKey, all)
}

func (m *Manager) Configured() map[string]Settings {
	all := m.settings()
	out := make(map[string]Settings, len(all))
	for name, settings := range all {
		out[name] = settings
	}
	return out
}

func (m *Manager) settings() map[string]Settings {
	if m == nil || m.Store == nil {
		return map[string]Settings{}
	}
	var out map[string]Settings
	m.Store.GetStruct(configKey, &out)
	if out == nil {
		return map[string]Settings{}
	}
	return out
}

func (m *Manager) profileRoot(name string, settings Settings) (string, error) {
	rootDir := strings.TrimSpace(m.RootDir)
	if rootDir == "" {
		return "", fmt.Errorf("missing profile root dir")
	}
	homePath := strings.TrimSpace(settings.HomePath)
	switch {
	case homePath == "":
		stateDir := strings.TrimSpace(m.StateDir)
		if stateDir == "" {
			stateDir = ".ctgbot"
		}
		return filepath.Join(rootDir, stateDir, "profiles", name), nil
	case filepath.IsAbs(homePath):
		return filepath.Clean(homePath), nil
	default:
		return filepath.Clean(filepath.Join(rootDir, homePath)), nil
	}
}

func validateName(label string, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("missing %s name", label)
	}
	if value == "." || value == ".." {
		return fmt.Errorf("invalid %s name: %q", label, value)
	}
	if strings.ContainsAny(value, `/\\`) {
		return fmt.Errorf("invalid %s name: %q", label, value)
	}
	if filepath.Clean(value) != value {
		return fmt.Errorf("invalid %s name: %q", label, value)
	}
	return nil
}
