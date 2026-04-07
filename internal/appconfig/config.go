package appconfig

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bartdeboer/go-clistate"
)

type Config struct {
	root  string
	Store *clistate.Store
}

const CodexLoginCallbackPort = 1455

const (
	stateDirName = ".ctgbot"
	namePrefix   = "ctgbot-"
)

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

func (c *Config) DBPath() string {
	return filepath.Join(c.Root(), "ctgbot.db")
}

func (c *Config) ChatsRoot() string {
	return filepath.Join(c.ProjectRoot(), "chats")
}

func (c *Config) ChatFolderName(chatID int64, threadID int) string {
	return fmt.Sprintf("%d-%d", chatID, threadID)
}

func (c *Config) ChatRuntimeName(chatID int64) string {
	return fmt.Sprintf("%d", chatID)
}

func (c *Config) ChatContainerName(chatID int64, threadID int) string {
	return fmt.Sprintf("%s%d-%d", namePrefix, chatID, threadID)
}

func (c *Config) ParseChatContainerName(name string) (chatID int64, threadID int, ok bool) {
	raw := strings.TrimPrefix(strings.TrimSpace(name), namePrefix)
	if raw == "" || raw == name {
		return 0, 0, false
	}
	idx := strings.LastIndex(raw, "-")
	if idx <= 0 || idx == len(raw)-1 {
		return 0, 0, false
	}
	chatID, err := strconv.ParseInt(raw[:idx], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	thread64, err := strconv.ParseInt(raw[idx+1:], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return chatID, int(thread64), true
}

func (c *Config) ChatRoot(name string) string {
	return filepath.Join(c.ChatsRoot(), name)
}

func (c *Config) ChatRuntimeRoot(chatID int64) string {
	return c.ChatRoot(c.ChatRuntimeName(chatID))
}

func (c *Config) ChatCodexHomeDir(name string) string {
	return filepath.Join(c.ChatRoot(name), ".codex")
}

func (c *Config) ChatCodexHomeDirByID(chatID int64) string {
	return filepath.Join(c.ChatRuntimeRoot(chatID), ".codex")
}

func (c *Config) ChatWorkspaceDir(name string) string {
	return filepath.Join(c.ChatRoot(name), "workspace")
}

func (c *Config) ChatWorkspaceDirByID(chatID int64) string {
	return filepath.Join(c.ChatRuntimeRoot(chatID), "workspace")
}

func (c *Config) ChatLogDir(name string) string {
	return filepath.Join(c.ChatRoot(name), "logs")
}

func (c *Config) ChatLogDirByID(chatID int64) string {
	return filepath.Join(c.ChatRuntimeRoot(chatID), "logs")
}

func (c *Config) ChatTLSDir(name string) string {
	return filepath.Join(c.ChatRoot(name), "tls")
}

func (c *Config) ChatTLSDirByID(chatID int64) string {
	return filepath.Join(c.ChatRuntimeRoot(chatID), "tls")
}

func (c *Config) ChatThreadsRoot(chatID int64) string {
	return filepath.Join(c.ChatRuntimeRoot(chatID), "threads")
}

func (c *Config) ChatThreadTLSDir(chatID int64, threadID int) string {
	return filepath.Join(c.ChatThreadsRoot(chatID), strconv.Itoa(threadID), "tls")
}

func (c *Config) HostbridgeTLSRoot() string {
	return filepath.Join(c.Root(), "tls")
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
		return "ctgbot-codex:latest"
	}
	return strings.TrimSpace(c.Store.GetString("docker.image", "ctgbot-codex:latest"))
}

func (c *Config) DockerCLIContainerName() string {
	if c == nil || c.Store == nil {
		return "ctgbot"
	}
	name := strings.TrimSpace(c.Store.GetString("docker.cli_container_name", "ctgbot"))
	if name == "" {
		return "ctgbot"
	}
	return name
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

func (c *Config) ChatWorkspaceHostPath(chatID int64) string {
	if c == nil || c.Store == nil {
		return ""
	}
	return absOrEmpty(c.Store.GetString(c.ChatKey(chatID, "workspace_host_path"), ""))
}

func (c *Config) SetChatWorkspaceHostPath(chatID int64, raw string) error {
	if c == nil || c.Store == nil {
		return fmt.Errorf("config store not available")
	}
	if chatID == 0 {
		return fmt.Errorf("chatID is 0")
	}
	resolved, err := c.ResolveWorkspaceHostPath(raw)
	if err != nil {
		return err
	}
	if c.Store.GetInt(c.ChatKey(chatID, "chat_id"), 0) == 0 {
		if err := c.Store.PersistInt(c.ChatKey(chatID, "chat_id"), int(chatID)); err != nil {
			return err
		}
	}
	return c.Store.PersistString(c.ChatKey(chatID, "workspace_host_path"), resolved)
}

func (c *Config) HostbridgeTCPListenAddr() string {
	if c == nil || c.Store == nil {
		return "127.0.0.1:4567"
	}
	v := strings.TrimSpace(c.Store.GetString("hostbridge.tcp_listen_addr", "127.0.0.1:4567"))
	if v == "" {
		return "127.0.0.1:4567"
	}
	return v
}

func (c *Config) ContainerHostbridgeTCPAddr() string {
	if c == nil || c.Store == nil {
		return "host.docker.internal:4567"
	}
	v := strings.TrimSpace(c.Store.GetString("docker.container_hostbridge_tcp_addr", "host.docker.internal:4567"))
	if v == "" {
		return "host.docker.internal:4567"
	}
	return v
}

func (c *Config) ChatHostbridgeAllowedCommandSpecs(chatID int64) []string {
	if c == nil || c.Store == nil {
		return nil
	}
	var out []string
	if !c.Store.GetStruct(c.ChatKey(chatID, "hostbridge.allowed_commands"), &out) {
		return nil
	}
	cleaned := make([]string, 0, len(out))
	for _, spec := range out {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		cleaned = append(cleaned, spec)
	}
	return cleaned
}

func (c *Config) SetChatHostbridgeAllowedCommand(chatID int64, spec string) error {
	if c == nil || c.Store == nil {
		return fmt.Errorf("config store not available")
	}
	if chatID == 0 {
		return fmt.Errorf("chatID is 0")
	}
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return fmt.Errorf("hostbridge allowed command spec is empty")
	}
	if c.Store.GetInt(c.ChatKey(chatID, "chat_id"), 0) == 0 {
		if err := c.Store.PersistInt(c.ChatKey(chatID, "chat_id"), int(chatID)); err != nil {
			return err
		}
	}
	commands := c.ChatHostbridgeAllowedCommandSpecs(chatID)
	for _, existing := range commands {
		if existing == spec {
			return nil
		}
	}
	commands = append(commands, spec)
	sort.Strings(commands)
	return c.Store.PersistStruct(c.ChatKey(chatID, "hostbridge.allowed_commands"), commands)
}

func (c *Config) RemoveChatHostbridgeAllowedCommand(chatID int64, name string) error {
	if c == nil || c.Store == nil {
		return fmt.Errorf("config store not available")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("hostbridge allowed command name is empty")
	}
	commands := c.ChatHostbridgeAllowedCommandSpecs(chatID)
	if len(commands) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(commands))
	for _, spec := range commands {
		if strings.EqualFold(filepath.Base(spec), name) {
			continue
		}
		filtered = append(filtered, spec)
	}
	return c.Store.PersistStruct(c.ChatKey(chatID, "hostbridge.allowed_commands"), filtered)
}

func (c *Config) ContainerWorkspacePath() string {
	if c == nil || c.Store == nil {
		return normalizeContainerPath("", "/workspace")
	}
	return normalizeContainerPath(c.Store.GetString("docker.container_workspace_path", "/workspace"), "/workspace")
}

func (c *Config) ContainerHomePath() string {
	if c == nil || c.Store == nil {
		return normalizeContainerPath("", "/codex-home")
	}
	return normalizeContainerPath(c.Store.GetString("docker.container_home_path", "/codex-home"), "/codex-home")
}

func (c *Config) ContainerHostbridgeTLSDir() string {
	if c == nil || c.Store == nil {
		return normalizeContainerPath("", "/etc/ctgbot/hostbridge-tls")
	}
	return normalizeContainerPath(c.Store.GetString("docker.container_hostbridge_tls_dir", "/etc/ctgbot/hostbridge-tls"), "/etc/ctgbot/hostbridge-tls")
}

func normalizeContainerPath(raw string, fallback string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		v = fallback
	}
	v = strings.ReplaceAll(v, "\\", "/")
	if !strings.HasPrefix(v, "/") {
		v = "/" + v
	}
	return path.Clean(v)
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

func (c *Config) CodexCLIHomeRoot() string {
	if c == nil {
		return ""
	}
	if c.Store != nil {
		if raw := absOrEmpty(c.Store.GetString("codex.cli_home_host_path", "")); raw != "" {
			return raw
		}
		if raw := absOrEmpty(c.Store.GetString("codex.shared_home_host_path", "")); raw != "" {
			return raw
		}
	}
	for _, root := range c.codexCLIHomeCandidates() {
		if fileExistsAndNonEmpty(filepath.Join(root, "auth.json")) {
			return root
		}
	}
	return c.LocalCodexCLIHomeRoot()
}

func (c *Config) LocalCodexCLIHomeRoot() string {
	if c == nil {
		return ""
	}
	return filepath.Join(c.Root(), ".codex")
}

func (c *Config) ManagedHomeCodexCLIHomeRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, stateDirName, ".codex")
}

func (c *Config) HostCodexRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

func (c *Config) EnsureCodexCLIHome() error {
	root := c.CodexCLIHomeRoot()
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("codex cli home root is empty")
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}

	if err := c.importAuthIfNeeded(); err != nil {
		return err
	}
	return nil
}

func (c *Config) importAuthIfNeeded() error {
	target := c.CodexCLIHomeAuthPath()
	if fileExistsAndNonEmpty(target) {
		return nil
	}
	for _, src := range c.CodexAuthSearchPaths() {
		if src == target {
			continue
		}
		if !fileExistsAndNonEmpty(src) {
			continue
		}
		if err := copyFile(src, target); err != nil {
			return err
		}
		return nil
	}
	return nil
}

func (c *Config) CodexCLIHomeAuthPath() string {
	return filepath.Join(c.CodexCLIHomeRoot(), "auth.json")
}

func (c *Config) CodexAuthSearchPaths() []string {
	roots := c.codexCLIHomeCandidates()
	out := make([]string, 0, len(roots))
	seen := map[string]struct{}{}
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		authPath := filepath.Join(root, "auth.json")
		if _, ok := seen[authPath]; ok {
			continue
		}
		seen[authPath] = struct{}{}
		out = append(out, authPath)
	}
	return out
}

func (c *Config) codexCLIHomeCandidates() []string {
	return []string{
		c.LocalCodexCLIHomeRoot(),
		c.ManagedHomeCodexCLIHomeRoot(),
		c.HostCodexRoot(),
	}
}

func (c *Config) ResolveChatWorkspaceHostPath(chatID int64, threadID int, raw string) (string, error) {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		candidate = c.ChatWorkspaceHostPath(chatID)
	}
	if candidate == "" {
		candidate = c.DefaultWorkspaceHostPath()
	}
	if candidate != "" {
		return c.ResolveWorkspaceHostPath(candidate)
	}
	workspace := c.ChatWorkspaceDirByID(chatID)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return "", err
	}
	return workspace, nil
}

func (c *Config) EnsureChatRuntimePaths(chatID int64) (string, error) {
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
