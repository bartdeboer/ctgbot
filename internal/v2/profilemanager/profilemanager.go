// Package profilemanager centralizes component profile paths.
//
// Profiles are named, reusable component directories. Chats can reference these
// profiles instead of owning credentials directly.
package profilemanager

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const profilesDir = "profiles"

type Manager struct {
	RootDir string
}

type Mount struct {
	HostPath      string
	ContainerPath string
}

func New(rootDir string) *Manager {
	return &Manager{RootDir: strings.TrimSpace(rootDir)}
}

func (m *Manager) HostPath(componentType string, profile string) (string, error) {
	root, err := m.rootDir()
	if err != nil {
		return "", err
	}
	componentType, err = cleanName("component type", componentType)
	if err != nil {
		return "", err
	}
	profile, err = cleanName("profile", profile)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, profilesDir, componentType, profile), nil
}

func (m *Manager) ContainerPath(componentType string) string {
	switch strings.TrimSpace(componentType) {
	case "codex":
		return "/codex-home"
	default:
		return "/profile"
	}
}

func (m *Manager) Ensure(componentType string, profile string) (string, error) {
	path, err := m.HostPath(componentType, profile)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", err
	}
	return path, nil
}

func (m *Manager) Mount(componentType string, profile string) (Mount, error) {
	hostPath, err := m.HostPath(componentType, profile)
	if err != nil {
		return Mount{}, err
	}
	return Mount{
		HostPath:      hostPath,
		ContainerPath: m.ContainerPath(componentType),
	}, nil
}

func (m *Manager) rootDir() (string, error) {
	if m == nil {
		return "", fmt.Errorf("missing profile manager")
	}
	root := strings.TrimSpace(m.RootDir)
	if root == "" {
		return "", fmt.Errorf("missing profile root")
	}
	return filepath.Join(root, ".ctgbot"), nil
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
