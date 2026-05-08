package appstate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func (c *Config) ChatsRoot() string {
	return filepath.Join(c.ProjectRoot(), "chats")
}

func (c *Config) EnsurePaths() error {
	for _, dir := range []string{c.RootDir(), c.ChatsRoot()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) EnsureChatRuntimePaths(chatID modeluuid.UUID) (string, error) {
	profile := c.Chat(chatID).Profile()
	if err := profile.EnsurePaths(); err != nil {
		return "", err
	}
	return profile.RuntimeName(), nil
}

func (c *Config) ResolveWorkspaceHostPath(raw string) (string, error) {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		candidate = c.Docker().DefaultWorkspaceHostPath()
	}
	if candidate == "" {
		return "", fmt.Errorf("missing workspace host path")
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace host path is not a directory: %s", abs)
	}
	return abs, nil
}
