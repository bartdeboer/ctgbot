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
	return c.migrateLegacyLocalLayout()
}

func (c *Config) EnsureChatRuntimePaths(chatID modeluuid.UUID) (string, error) {
	profile := c.Chat(chatID).Profile()
	if err := profile.EnsurePaths(); err != nil {
		return "", err
	}
	return profile.RuntimeName(), nil
}

func (c *Config) migrateLegacyLocalLayout() error {
	legacyRoot := filepath.Join(c.RootDir(), "conversations")
	entries, err := os.ReadDir(legacyRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(c.ChatsRoot(), 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		key := strings.TrimPrefix(name, "ctgbot-")
		if key == name {
			continue
		}
		srcRoot := filepath.Join(legacyRoot, name)
		dstRoot := filepath.Join(c.ChatsRoot(), key)
		if !pathExists(dstRoot) {
			if err := os.Rename(srcRoot, dstRoot); err != nil {
				return err
			}
		}
		oldHome := filepath.Join(dstRoot, "home")
		newHome := filepath.Join(dstRoot, ".codex")
		if pathExists(oldHome) && !pathExists(newHome) {
			if err := os.Rename(oldHome, newHome); err != nil {
				return err
			}
		}
		if err := os.MkdirAll(filepath.Join(dstRoot, "workspace"), 0o755); err != nil {
			return err
		}
	}
	remaining, err := os.ReadDir(legacyRoot)
	if err == nil && len(remaining) == 0 {
		if err := os.Remove(legacyRoot); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
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
