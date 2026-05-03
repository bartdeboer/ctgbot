package homes

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	v4component "github.com/bartdeboer/ctgbot/internal/v4/component"
	"github.com/bartdeboer/ctgbot/internal/v4/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v4/profiles"
)

const (
	defaultComponentsDir = "components"
	defaultContainerRoot = "/profile"
)

type Manager struct {
	Profiles      *profiles.Manager
	ComponentsDir string
	ContainerRoot string
}

func New(profileManager *profiles.Manager) *Manager {
	return &Manager{
		Profiles:      profileManager,
		ComponentsDir: defaultComponentsDir,
		ContainerRoot: defaultContainerRoot,
	}
}

func (m *Manager) Home(component coremodel.Component) (v4component.Home, error) {
	if m == nil || m.Profiles == nil {
		return v4component.Home{}, fmt.Errorf("missing home manager")
	}
	profileName := strings.TrimSpace(component.Profile)
	if profileName == "" {
		profileName = "default"
	}
	profile, err := m.Profiles.Resolve(profileName)
	if err != nil {
		return v4component.Home{}, err
	}
	componentType, err := cleanName("component type", component.Type)
	if err != nil {
		return v4component.Home{}, err
	}
	name, err := cleanName("component name", component.Name)
	if err != nil {
		return v4component.Home{}, err
	}
	componentsDir := strings.TrimSpace(m.ComponentsDir)
	if componentsDir == "" {
		componentsDir = defaultComponentsDir
	}
	containerRoot := strings.TrimSpace(m.ContainerRoot)
	if containerRoot == "" {
		containerRoot = defaultContainerRoot
	}
	return v4component.Home{
		HostPath:      filepath.Join(profile.Root, componentsDir, componentType, name),
		ContainerPath: pathJoin(containerRoot, componentsDir, componentType, name),
	}, nil
}

func (m *Manager) Ensure(component coremodel.Component) (v4component.Home, error) {
	home, err := m.Home(component)
	if err != nil {
		return v4component.Home{}, err
	}
	if err := os.MkdirAll(home.HostPath, 0o700); err != nil {
		return v4component.Home{}, err
	}
	return home, nil
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
