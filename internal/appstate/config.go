package appstate

import (
	"context"
	"fmt"
	"github.com/bartdeboer/ctgbot/internal/durationparse"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/go-clistate"
)

type Config struct {
	root  string
	Store *clistate.Store
}

const CodexLoginCallbackPort = 1455

const (
	stateDirName         = ".ctgbot"
	namePrefix           = "ctgbot-"
	chatClientNamePrefix = "ctgbot-chat-"
)

type ChatConfigEntry struct {
	ID                          modeluuid.UUID
	ProviderType                string
	ProviderChatID              string
	ProviderChatTitle           string
	Enabled                     bool
	InteractiveInterruptEnabled bool
}

type GitIdentity struct {
	Name  string
	Email string
}

func NewConfig(root string, store *clistate.Store) (*Config, error) {
	if strings.TrimSpace(root) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(cwd, stateDirName)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	return &Config{
		root:  absRoot,
		Store: store,
	}, nil
}

func (c *Config) Root() string {
	return c.root
}

func (c *Config) ProjectRoot() string {
	if c == nil {
		return ""
	}
	return filepath.Dir(c.Root())
}

func (c *Config) EnsurePaths() error {
	for _, dir := range []string{c.Root(), c.ChatsRoot()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := c.migrateLegacyLocalLayout(); err != nil {
		return err
	}
	return nil
}

func (c *Config) EnsureChatRuntimePaths(chatID modeluuid.UUID) (string, error) {
	name := c.ChatRuntimeName(chatID)
	for _, dir := range []string{
		c.ChatRuntimeRoot(chatID),
		c.ChatCodexHomeDirByID(chatID),
		c.ChatWorkspaceDirByID(chatID),
		c.ChatLogDirByID(chatID),
		c.ChatTLSDirByID(chatID),
		c.ChatThreadsRoot(chatID),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	if err := ensureGitWorkspace(c.ChatWorkspaceDirByID(chatID)); err != nil {
		return "", err
	}
	return name, nil
}

func (c *Config) migrateLegacyLocalLayout() error {
	legacyRoot := filepath.Join(c.Root(), "conversations")
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
		key := strings.TrimPrefix(name, namePrefix)
		if key == name {
			continue
		}

		srcRoot := filepath.Join(legacyRoot, name)
		dstRoot := c.ChatRoot(key)
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
		candidate = c.DefaultWorkspaceHostPath()
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

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fileExistsAndNonEmpty(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Size() > 0
}

func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("expected file, got directory: %s", src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return os.Chmod(dst, info.Mode().Perm())
}

func ensureGitWorkspace(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("workspace dir is empty")
	}
	if pathExists(filepath.Join(dir, ".git")) {
		return nil
	}
	cmd := exec.Command("git", "init", "-q", dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git init %s: %w: %s", dir, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func absOrEmpty(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	abs, err := filepath.Abs(v)
	if err != nil {
		return v
	}
	return abs
}

func readGitConfig(ctx context.Context, key string) string {
	if strings.TrimSpace(key) == "" {
		return ""
	}
	if ctx == nil {
		ctx = context.Background()
	}

	cmd := exec.CommandContext(ctx, "git", "config", "--global", key)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func stringFromAny(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if s := stringFromAny(value); s != "" {
			return s
		}
	}
	return ""
}

func boolFromAny(v any) bool {
	b, _ := v.(bool)
	return b
}

func (c *Config) durationFromConfig(key string, fallback int, unit time.Duration) time.Duration {
	if c == nil || c.Store == nil {
		return time.Duration(fallback) * unit
	}
	if raw := strings.TrimSpace(c.Store.GetString(key, "")); raw != "" {
		d, err := durationparse.Parse(raw, unit)
		if err == nil {
			return d
		}
	}
	return time.Duration(c.Store.GetInt(key, fallback)) * unit
}
