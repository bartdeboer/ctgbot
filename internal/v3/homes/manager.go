package homes

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	v3component "github.com/bartdeboer/ctgbot/internal/v3/component"
	"github.com/bartdeboer/ctgbot/internal/v3/coremodel"
)

const (
	defaultStateDir      = ".ctgbot"
	defaultComponentsDir = "components"
	defaultContainerRoot = "/components"
)

type Manager struct {
	RootDir       string
	StateDir      string
	ComponentsDir string
	ContainerRoot string
}

func New(rootDir string) *Manager {
	return &Manager{
		RootDir:       strings.TrimSpace(rootDir),
		StateDir:      defaultStateDir,
		ComponentsDir: defaultComponentsDir,
		ContainerRoot: defaultContainerRoot,
	}
}

func (m *Manager) Home(component coremodel.Component) (v3component.Home, error) {
	hostRoot, err := m.hostRoot()
	if err != nil {
		return v3component.Home{}, err
	}
	componentType, err := cleanName("component type", component.Type)
	if err != nil {
		return v3component.Home{}, err
	}
	name, err := cleanName("component name", component.Name)
	if err != nil {
		return v3component.Home{}, err
	}
	containerRoot := strings.TrimSpace(m.ContainerRoot)
	if containerRoot == "" {
		containerRoot = defaultContainerRoot
	}
	return v3component.Home{
		HostPath:      filepath.Join(hostRoot, componentType, name),
		ContainerPath: pathJoin(containerRoot, componentType, name),
	}, nil
}

func (m *Manager) Ensure(component coremodel.Component) (v3component.Home, error) {
	home, err := m.Home(component)
	if err != nil {
		return v3component.Home{}, err
	}
	if err := os.MkdirAll(home.HostPath, 0o700); err != nil {
		return v3component.Home{}, err
	}
	return home, nil
}

func (m *Manager) hostRoot() (string, error) {
	if m == nil {
		return "", fmt.Errorf("missing home manager")
	}
	root := strings.TrimSpace(m.RootDir)
	if root == "" {
		return "", fmt.Errorf("missing home root")
	}
	stateDir := strings.TrimSpace(m.StateDir)
	if stateDir == "" {
		stateDir = defaultStateDir
	}
	componentsDir := strings.TrimSpace(m.ComponentsDir)
	if componentsDir == "" {
		componentsDir = defaultComponentsDir
	}
	return filepath.Join(root, stateDir, componentsDir), nil
}

func cleanName(label string, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("missing %s", label)
	}
	if value == "." || value == ".." {
		return "", fmt.Errorf("invalid %s: %q", label, value)
	}
	if strings.ContainsAny(value, `/\\`) {
		return "", fmt.Errorf("invalid %s: %q", label, value)
	}
	if filepath.Clean(value) != value {
		return "", fmt.Errorf("invalid %s: %q", label, value)
	}
	return value, nil
}

func pathJoin(root string, values ...string) string {
	root = strings.TrimRight(strings.TrimSpace(root), "/")
	if root == "" {
		root = "/"
	}
	if root == "/" {
		return "/" + strings.Join(values, "/")
	}
	return root + "/" + strings.Join(values, "/")
}
