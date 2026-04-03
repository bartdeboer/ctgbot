package botengine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bartdeboer/go-clistate"
)

type Config struct {
	root  string
	Store *clistate.Store
}

type ChatConfigEntry struct {
	ChatID    int64
	ChatTitle string
	Enabled   bool
	Scope     string
}

func NewConfig(root string, store *clistate.Store) (*Config, error) {
	if strings.TrimSpace(root) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(cwd, ".codextgbot")
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

func (c *Config) EnsurePaths() error {
	for _, dir := range []string{c.Root(), c.ConversationsRoot()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) DBPath() string {
	return filepath.Join(c.Root(), "codextgbot.db")
}

func (c *Config) ConversationsRoot() string {
	return filepath.Join(c.Root(), "conversations")
}

func (c *Config) ConversationRoot(name string) string {
	return filepath.Join(c.ConversationsRoot(), name)
}

func (c *Config) ConversationHomeDir(name string) string {
	return filepath.Join(c.ConversationRoot(name), "home")
}

func (c *Config) ConversationLogDir(name string) string {
	return filepath.Join(c.ConversationRoot(name), "logs")
}

func (c *Config) TelegramToken() string {
	if c == nil || c.Store == nil {
		return ""
	}
	return strings.TrimSpace(c.Store.GetString("telegram.token", ""))
}

func (c *Config) PollTimeout() time.Duration {
	sec := 60
	if c.Store != nil {
		sec = c.Store.GetInt("telegram.defaults.poll_timeout_sec", sec)
	}
	return time.Duration(sec) * time.Second
}

func (c *Config) SessionTimeout() time.Duration {
	minutes := 10
	if c.Store != nil {
		minutes = c.Store.GetInt("session.timeout_min", minutes)
	}
	return time.Duration(minutes) * time.Minute
}

func (c *Config) DockerImage() string {
	if c == nil || c.Store == nil {
		return "codextgbot-codex:latest"
	}
	return strings.TrimSpace(c.Store.GetString("docker.image", "codextgbot-codex:latest"))
}

func (c *Config) ProjectDir() string {
	if c == nil || c.Store == nil {
		return ""
	}
	return strings.TrimSpace(c.Store.GetProjectDir())
}

func (c *Config) DefaultWorkspaceHostPath() string {
	if c == nil || c.Store == nil {
		return ""
	}
	return absOrEmpty(c.Store.GetString("docker.workspace_host_path", ""))
}

func (c *Config) HostbridgeSocketPath() string {
	if c == nil || c.Store == nil {
		return ""
	}
	return absOrEmpty(c.Store.GetString("docker.hostbridge_socket_path", ""))
}

func (c *Config) ContainerWorkspacePath() string {
	if c == nil || c.Store == nil {
		return "/workspace"
	}
	v := strings.TrimSpace(c.Store.GetString("docker.container_workspace_path", "/workspace"))
	if v == "" {
		return "/workspace"
	}
	return v
}

func (c *Config) ContainerHomePath() string {
	if c == nil || c.Store == nil {
		return "/codex-home"
	}
	v := strings.TrimSpace(c.Store.GetString("docker.container_home_path", "/codex-home"))
	if v == "" {
		return "/codex-home"
	}
	return v
}

func (c *Config) ContainerSharedCodexPath() string {
	if c == nil || c.Store == nil {
		return "/codex-shared"
	}
	v := strings.TrimSpace(c.Store.GetString("docker.container_shared_codex_path", "/codex-shared"))
	if v == "" {
		return "/codex-shared"
	}
	return v
}

func (c *Config) ContainerHostbridgeSocketPath() string {
	if c == nil || c.Store == nil {
		return "/run/hostbridge/bridge.sock"
	}
	v := strings.TrimSpace(c.Store.GetString("docker.container_hostbridge_socket_path", "/run/hostbridge/bridge.sock"))
	if v == "" {
		return "/run/hostbridge/bridge.sock"
	}
	return v
}

func (c *Config) CodexModel() string {
	if c == nil || c.Store == nil {
		return ""
	}
	return strings.TrimSpace(c.Store.GetString("codex.model", ""))
}

func (c *Config) CodexFullAuto() bool {
	if c == nil || c.Store == nil {
		return true
	}
	return c.Store.GetBool("codex.full_auto", true)
}

func (c *Config) SharedCodexRoot() string {
	if c == nil {
		return ""
	}
	if c.Store != nil {
		if raw := absOrEmpty(c.Store.GetString("codex.shared_home_host_path", "")); raw != "" {
			return raw
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codextgbot", "codex-shared")
}

func (c *Config) EnsureSharedCodexPaths() error {
	root := c.SharedCodexRoot()
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("shared codex root is empty")
	}
	for _, dir := range []string{
		root,
		filepath.Join(root, "cache"),
		filepath.Join(root, "memories"),
		filepath.Join(root, "skills"),
		filepath.Join(root, "vendor_imports"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
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
		return "", fmt.Errorf("missing workspace host path; configure `docker.workspace_host_path` or use /new <absolute-path>")
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

func (c *Config) chatScopeAndID(chatID int64) (scope string, id int64) {
	if chatID < 0 {
		return "groups", -chatID
	}
	return "users", chatID
}

func (c *Config) ChatKey(chatID int64, key string) string {
	scope, id := c.chatScopeAndID(chatID)
	return fmt.Sprintf("telegram.chats.%s.%d.%s", scope, id, key)
}

func (c *Config) PersistChatID(chatID int64, chatTitle string) error {
	if c == nil || c.Store == nil || chatID == 0 {
		return fmt.Errorf("chatID is 0")
	}

	existing := c.Store.GetInt(c.ChatKey(chatID, "chat_id"), 0)
	if existing != 0 {
		return nil
	}

	if err := c.Store.PersistInt(c.ChatKey(chatID, "chat_id"), int(chatID)); err != nil {
		return err
	}
	if err := c.Store.PersistBool(c.ChatKey(chatID, "enabled"), false); err != nil {
		return err
	}
	return c.Store.PersistString(c.ChatKey(chatID, "chat_title"), strings.TrimSpace(chatTitle))
}

func (c *Config) ChatEnabled(chatID int64) bool {
	if c == nil || c.Store == nil {
		return false
	}
	if v := c.Store.Get(c.ChatKey(chatID, "enabled"), nil); v != nil {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func (c *Config) SetChatEnabled(chatID int64, enabled bool) error {
	if c == nil || c.Store == nil {
		return fmt.Errorf("config store not available")
	}
	if chatID == 0 {
		return fmt.Errorf("chatID is 0")
	}
	if c.Store.GetInt(c.ChatKey(chatID, "chat_id"), 0) == 0 {
		if err := c.Store.PersistInt(c.ChatKey(chatID, "chat_id"), int(chatID)); err != nil {
			return err
		}
	}
	return c.Store.PersistBool(c.ChatKey(chatID, "enabled"), enabled)
}

func (c *Config) KnownChats() []ChatConfigEntry {
	if c == nil || c.Store == nil {
		return nil
	}

	rootAny := c.Store.Get("telegram.chats", nil)
	root, ok := rootAny.(map[string]any)
	if !ok {
		return nil
	}

	var out []ChatConfigEntry
	appendScope := func(scope string, scopeMap map[string]any) {
		for _, raw := range scopeMap {
			entryMap, ok := raw.(map[string]any)
			if !ok {
				continue
			}

			chatID := int64FromAny(entryMap["chat_id"])
			if chatID == 0 && scope == "groups" {
				chatID = -int64FromAny(entryMap["id"])
			}
			if chatID == 0 {
				continue
			}

			out = append(out, ChatConfigEntry{
				ChatID:    chatID,
				ChatTitle: stringFromAny(entryMap["chat_title"]),
				Enabled:   boolFromAny(entryMap["enabled"]),
				Scope:     scope,
			})
		}
	}

	if users, ok := root["users"].(map[string]any); ok {
		appendScope("users", users)
	}
	if groups, ok := root["groups"].(map[string]any); ok {
		appendScope("groups", groups)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Scope != out[j].Scope {
			return out[i].Scope < out[j].Scope
		}
		if out[i].Enabled != out[j].Enabled {
			return out[i].Enabled && !out[j].Enabled
		}
		if out[i].ChatTitle != out[j].ChatTitle {
			return out[i].ChatTitle < out[j].ChatTitle
		}
		return out[i].ChatID < out[j].ChatID
	})

	return out
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

func stringFromAny(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func boolFromAny(v any) bool {
	b, _ := v.(bool)
	return b
}

func int64FromAny(v any) int64 {
	switch t := v.(type) {
	case int:
		return int64(t)
	case int64:
		return t
	case float64:
		return int64(t)
	case float32:
		return int64(t)
	default:
		return 0
	}
}
