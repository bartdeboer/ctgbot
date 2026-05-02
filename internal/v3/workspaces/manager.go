package workspaces

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

const DefaultContainerWorkspace = "/workspace"

type Manager struct {
	rootDir            string
	containerWorkspace string
}

func New(rootDir string) *Manager {
	return &Manager{
		rootDir:            strings.TrimSpace(rootDir),
		containerWorkspace: DefaultContainerWorkspace,
	}
}

func (m *Manager) ContainerWorkspace() string {
	if m == nil || strings.TrimSpace(m.containerWorkspace) == "" {
		return DefaultContainerWorkspace
	}
	return m.containerWorkspace
}

func (m *Manager) HostPath(threadID modeluuid.UUID) (string, error) {
	if m == nil || strings.TrimSpace(m.rootDir) == "" {
		return "", fmt.Errorf("missing workspace root")
	}
	if threadID.IsNull() {
		return "", fmt.Errorf("missing thread id")
	}
	return filepath.Join(m.rootDir, ".ctgbot", "threads", threadID.String(), "workspace"), nil
}

func (m *Manager) Ensure(threadID modeluuid.UUID) (string, error) {
	hostPath, err := m.HostPath(threadID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(hostPath, 0o755); err != nil {
		return "", err
	}
	inbox := filepath.Join(hostPath, "inbox")
	if err := os.MkdirAll(inbox, 0o755); err != nil {
		return "", err
	}
	return hostPath, nil
}
