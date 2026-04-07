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

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
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
	ID                modeluuid.UUID
	ChatProviderType  string
	ProviderChatID    string
	ProviderChatTitle string
	Enabled           bool
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
	if err := c.migrateLegacyChatConfig(); err != nil {
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

func (c *Config) ChatRuntimeName(chatID modeluuid.UUID) string {
	return chatID.String()
}

func (c *Config) ChatContainerName(chatID modeluuid.UUID, threadID modeluuid.UUID) string {
	return fmt.Sprintf("%s%s-%s", namePrefix, chatID.String(), threadID.String())
}

func (c *Config) ParseChatContainerName(name string) (chatID modeluuid.UUID, threadID modeluuid.UUID, ok bool) {
	raw := strings.TrimPrefix(strings.TrimSpace(name), namePrefix)
	if raw == "" || raw == name {
		return modeluuid.Nil, modeluuid.Nil, false
	}

	parts := strings.SplitN(raw, "-", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return modeluuid.Nil, modeluuid.Nil, false
	}

	chatID, err := modeluuid.Parse(parts[0])
	if err != nil {
		return modeluuid.Nil, modeluuid.Nil, false
	}
	threadID, err = modeluuid.Parse(parts[1])
	if err != nil {
		return modeluuid.Nil, modeluuid.Nil, false
	}
	return chatID, threadID, true
}

func (c *Config) ChatRoot(name string) string {
	return filepath.Join(c.ChatsRoot(), name)
}

func (c *Config) ChatRuntimeRoot(chatID modeluuid.UUID) string {
	return c.ChatRoot(c.ChatRuntimeName(chatID))
}

func (c *Config) ChatCodexHomeDir(name string) string {
	return filepath.Join(c.ChatRoot(name), ".codex")
}

func (c *Config) ChatCodexHomeDirByID(chatID modeluuid.UUID) string {
	return filepath.Join(c.ChatRuntimeRoot(chatID), ".codex")
}

func (c *Config) ChatWorkspaceDir(name string) string {
	return filepath.Join(c.ChatRoot(name), "workspace")
}

func (c *Config) ChatWorkspaceDirByID(chatID modeluuid.UUID) string {
	return filepath.Join(c.ChatRuntimeRoot(chatID), "workspace")
}

func (c *Config) ChatLogDir(name string) string {
	return filepath.Join(c.ChatRoot(name), "logs")
}

func (c *Config) ChatLogDirByID(chatID modeluuid.UUID) string {
	return filepath.Join(c.ChatRuntimeRoot(chatID), "logs")
}

func (c *Config) ChatTLSDir(name string) string {
	return filepath.Join(c.ChatRoot(name), "tls")
}

func (c *Config) ChatTLSDirByID(chatID modeluuid.UUID) string {
	return filepath.Join(c.ChatRuntimeRoot(chatID), "tls")
}

func (c *Config) ChatThreadsRoot(chatID modeluuid.UUID) string {
	return filepath.Join(c.ChatRuntimeRoot(chatID), "threads")
}

func (c *Config) ChatThreadTLSDir(chatID modeluuid.UUID, threadID modeluuid.UUID) string {
	return filepath.Join(c.ChatThreadsRoot(chatID), threadID.String(), "tls")
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
	return c.importAuthIfNeeded()
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
		return copyFile(src, target)
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

func (c *Config) FindChatByID(chatID modeluuid.UUID) (*ChatConfigEntry, error) {
	if err := c.migrateLegacyChatConfig(); err != nil {
		return nil, err
	}
	return c.findChatByIDNoMigrate(chatID), nil
}

func (c *Config) findChatByIDNoMigrate(chatID modeluuid.UUID) *ChatConfigEntry {
	if c == nil || c.Store == nil || chatID.IsNull() {
		return nil
	}
	if !c.hasChatConfigByID(chatID) {
		return nil
	}
	return &ChatConfigEntry{
		ID:                chatID,
		ChatProviderType:  strings.TrimSpace(c.Store.GetString(c.ChatKey(chatID, "chat_provider_type"), c.Store.GetString(c.ChatKey(chatID, "type"), ""))),
		ProviderChatID:    strings.TrimSpace(c.Store.GetString(c.ChatKey(chatID, "provider_chat_id"), "")),
		ProviderChatTitle: strings.TrimSpace(c.Store.GetString(c.ChatKey(chatID, "provider_chat_title"), "")),
		Enabled:           c.Store.GetBool(c.ChatKey(chatID, "enabled"), false),
	}
}

func (c *Config) hasChatConfigByID(chatID modeluuid.UUID) bool {
	if c == nil || c.Store == nil || chatID.IsNull() {
		return false
	}
	for _, key := range []string{"chat_provider_type", "type", "provider_chat_id", "provider_chat_title", "enabled"} {
		if c.Store.Get(c.ChatKey(chatID, key), nil) != nil {
			return true
		}
	}
	return false
}

func (c *Config) FindProviderChat(providerType string, providerChatID string) (*ChatConfigEntry, error) {
	if err := c.migrateLegacyChatConfig(); err != nil {
		return nil, err
	}
	return c.findProviderChatNoMigrate(providerType, providerChatID), nil
}

func (c *Config) findProviderChatNoMigrate(providerType string, providerChatID string) *ChatConfigEntry {
	providerType = strings.TrimSpace(providerType)
	providerChatID = strings.TrimSpace(providerChatID)
	if c == nil || c.Store == nil || providerType == "" || providerChatID == "" {
		return nil
	}

	if rawID := strings.TrimSpace(c.Store.GetString(c.providerChatMapKey(providerType, providerChatID), "")); rawID != "" {
		if id, err := modeluuid.Parse(rawID); err == nil {
			if entry := c.findChatByIDNoMigrate(id); entry != nil {
				return entry
			}
		}
	}

	for _, chat := range c.knownChatsNoMigrate() {
		if chat.ChatProviderType == providerType && chat.ProviderChatID == providerChatID {
			entry := chat
			return &entry
		}
	}
	return nil
}

func (c *Config) EnsureProviderChat(providerType string, providerChatID string, title string) (*ChatConfigEntry, error) {
	if err := c.migrateLegacyChatConfig(); err != nil {
		return nil, err
	}
	if c == nil || c.Store == nil {
		return nil, fmt.Errorf("config store not available")
	}

	providerType = strings.TrimSpace(providerType)
	providerChatID = strings.TrimSpace(providerChatID)
	title = strings.TrimSpace(title)
	if providerType == "" {
		return nil, fmt.Errorf("provider type is empty")
	}
	if providerChatID == "" {
		return nil, fmt.Errorf("provider chat id is empty")
	}

	existing, err := c.FindProviderChat(providerType, providerChatID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if title != "" && existing.ProviderChatTitle != title {
			if err := c.Store.PersistString(c.ChatKey(existing.ID, "provider_chat_title"), title); err != nil {
				return nil, err
			}
			existing.ProviderChatTitle = title
		}
		return existing, nil
	}

	entry := &ChatConfigEntry{
		ID:                modeluuid.New(),
		ChatProviderType:  providerType,
		ProviderChatID:    providerChatID,
		ProviderChatTitle: title,
		Enabled:           false,
	}

	if err := c.Store.PersistString(c.ChatKey(entry.ID, "chat_provider_type"), entry.ChatProviderType); err != nil {
		return nil, err
	}
	if err := c.Store.PersistString(c.ChatKey(entry.ID, "provider_chat_id"), entry.ProviderChatID); err != nil {
		return nil, err
	}
	if err := c.Store.PersistString(c.ChatKey(entry.ID, "provider_chat_title"), entry.ProviderChatTitle); err != nil {
		return nil, err
	}
	if err := c.Store.PersistBool(c.ChatKey(entry.ID, "enabled"), entry.Enabled); err != nil {
		return nil, err
	}
	if err := c.Store.PersistString(c.providerChatMapKey(entry.ChatProviderType, entry.ProviderChatID), entry.ID.String()); err != nil {
		return nil, err
	}
	return entry, nil
}

func (c *Config) SetChatEnabledByID(chatID modeluuid.UUID, enabled bool) error {
	if err := c.migrateLegacyChatConfig(); err != nil {
		return err
	}
	if c == nil || c.Store == nil {
		return fmt.Errorf("config store not available")
	}
	if chatID.IsNull() {
		return fmt.Errorf("chat id is null")
	}
	return c.Store.PersistBool(c.ChatKey(chatID, "enabled"), enabled)
}

func (c *Config) ChatEnabledByID(chatID modeluuid.UUID) bool {
	if err := c.migrateLegacyChatConfig(); err != nil {
		return false
	}
	if c == nil || c.Store == nil || chatID.IsNull() {
		return false
	}
	return c.Store.GetBool(c.ChatKey(chatID, "enabled"), false)
}

func (c *Config) ChatWorkspaceHostPathByID(chatID modeluuid.UUID) string {
	if err := c.migrateLegacyChatConfig(); err != nil {
		return ""
	}
	if c == nil || c.Store == nil || chatID.IsNull() {
		return ""
	}
	return absOrEmpty(c.Store.GetString(c.ChatKey(chatID, "workspace_host_path"), ""))
}

func (c *Config) SetChatWorkspaceHostPathByID(chatID modeluuid.UUID, raw string) error {
	if err := c.migrateLegacyChatConfig(); err != nil {
		return err
	}
	if c == nil || c.Store == nil {
		return fmt.Errorf("config store not available")
	}
	if chatID.IsNull() {
		return fmt.Errorf("chat id is null")
	}
	resolved, err := c.ResolveWorkspaceHostPath(raw)
	if err != nil {
		return err
	}
	return c.Store.PersistString(c.ChatKey(chatID, "workspace_host_path"), resolved)
}

func (c *Config) ChatHostbridgeAllowedCommandSpecsByID(chatID modeluuid.UUID) []string {
	if err := c.migrateLegacyChatConfig(); err != nil {
		return nil
	}
	if c == nil || c.Store == nil || chatID.IsNull() {
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

func (c *Config) SetChatHostbridgeAllowedCommandByID(chatID modeluuid.UUID, spec string) error {
	if err := c.migrateLegacyChatConfig(); err != nil {
		return err
	}
	if c == nil || c.Store == nil {
		return fmt.Errorf("config store not available")
	}
	if chatID.IsNull() {
		return fmt.Errorf("chat id is null")
	}
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return fmt.Errorf("hostbridge allowed command spec is empty")
	}

	commands := c.ChatHostbridgeAllowedCommandSpecsByID(chatID)
	for _, existing := range commands {
		if existing == spec {
			return nil
		}
	}
	commands = append(commands, spec)
	sort.Strings(commands)
	return c.Store.PersistStruct(c.ChatKey(chatID, "hostbridge.allowed_commands"), commands)
}

func (c *Config) RemoveChatHostbridgeAllowedCommandByID(chatID modeluuid.UUID, name string) error {
	if err := c.migrateLegacyChatConfig(); err != nil {
		return err
	}
	if c == nil || c.Store == nil {
		return fmt.Errorf("config store not available")
	}
	if chatID.IsNull() {
		return fmt.Errorf("chat id is null")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("hostbridge allowed command name is empty")
	}

	commands := c.ChatHostbridgeAllowedCommandSpecsByID(chatID)
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

func (c *Config) ResolveChatWorkspaceHostPathByID(chatID modeluuid.UUID, raw string) (string, error) {
	if err := c.migrateLegacyChatConfig(); err != nil {
		return "", err
	}
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		candidate = c.ChatWorkspaceHostPathByID(chatID)
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

func (c *Config) KnownChats() []ChatConfigEntry {
	if err := c.migrateLegacyChatConfig(); err != nil {
		return nil
	}
	return c.knownChatsNoMigrate()
}

func (c *Config) knownChatsNoMigrate() []ChatConfigEntry {
	if c == nil || c.Store == nil {
		return nil
	}
	rootAny := c.Store.Get("chats", nil)
	root, ok := rootAny.(map[string]any)
	if !ok {
		return nil
	}

	out := make([]ChatConfigEntry, 0, len(root))
	for rawID, rawEntry := range root {
		entryMap, ok := rawEntry.(map[string]any)
		if !ok {
			continue
		}
		id, err := modeluuid.Parse(strings.TrimSpace(rawID))
		if err != nil {
			continue
		}
		out = append(out, ChatConfigEntry{
			ID:                id,
			ChatProviderType:  firstNonEmptyString(entryMap["chat_provider_type"], entryMap["type"]),
			ProviderChatID:    stringFromAny(entryMap["provider_chat_id"]),
			ProviderChatTitle: stringFromAny(entryMap["provider_chat_title"]),
			Enabled:           boolFromAny(entryMap["enabled"]),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].ChatProviderType != out[j].ChatProviderType {
			return out[i].ChatProviderType < out[j].ChatProviderType
		}
		if out[i].Enabled != out[j].Enabled {
			return out[i].Enabled && !out[j].Enabled
		}
		if out[i].ProviderChatTitle != out[j].ProviderChatTitle {
			return out[i].ProviderChatTitle < out[j].ProviderChatTitle
		}
		if out[i].ProviderChatID != out[j].ProviderChatID {
			return out[i].ProviderChatID < out[j].ProviderChatID
		}
		return out[i].ID.String() < out[j].ID.String()
	})
	return out
}

func (c *Config) ChatKey(chatID modeluuid.UUID, key string) string {
	base := fmt.Sprintf("chats.%s", chatID.String())
	if strings.TrimSpace(key) == "" {
		return base
	}
	return base + "." + key
}

func (c *Config) PersistChatID(chatID int64, chatTitle string) error {
	_, err := c.EnsureProviderChat("telegram", strconv.FormatInt(chatID, 10), chatTitle)
	return err
}

func (c *Config) ChatEnabled(chatID int64) bool {
	entry, err := c.FindProviderChat("telegram", strconv.FormatInt(chatID, 10))
	if err != nil || entry == nil {
		return false
	}
	return entry.Enabled
}

func (c *Config) SetChatEnabled(chatID int64, enabled bool) error {
	entry, err := c.EnsureProviderChat("telegram", strconv.FormatInt(chatID, 10), "")
	if err != nil {
		return err
	}
	return c.SetChatEnabledByID(entry.ID, enabled)
}

func (c *Config) ChatWorkspaceHostPath(chatID int64) string {
	entry, err := c.FindProviderChat("telegram", strconv.FormatInt(chatID, 10))
	if err != nil || entry == nil {
		return ""
	}
	return c.ChatWorkspaceHostPathByID(entry.ID)
}

func (c *Config) SetChatWorkspaceHostPath(chatID int64, raw string) error {
	entry, err := c.EnsureProviderChat("telegram", strconv.FormatInt(chatID, 10), "")
	if err != nil {
		return err
	}
	return c.SetChatWorkspaceHostPathByID(entry.ID, raw)
}

func (c *Config) ChatHostbridgeAllowedCommandSpecs(chatID int64) []string {
	entry, err := c.FindProviderChat("telegram", strconv.FormatInt(chatID, 10))
	if err != nil || entry == nil {
		return nil
	}
	return c.ChatHostbridgeAllowedCommandSpecsByID(entry.ID)
}

func (c *Config) SetChatHostbridgeAllowedCommand(chatID int64, spec string) error {
	entry, err := c.EnsureProviderChat("telegram", strconv.FormatInt(chatID, 10), "")
	if err != nil {
		return err
	}
	return c.SetChatHostbridgeAllowedCommandByID(entry.ID, spec)
}

func (c *Config) RemoveChatHostbridgeAllowedCommand(chatID int64, name string) error {
	entry, err := c.FindProviderChat("telegram", strconv.FormatInt(chatID, 10))
	if err != nil || entry == nil {
		return nil
	}
	return c.RemoveChatHostbridgeAllowedCommandByID(entry.ID, name)
}

func (c *Config) ResolveChatWorkspaceHostPath(chatID int64, threadID int, raw string) (string, error) {
	_ = threadID
	entry, err := c.FindProviderChat("telegram", strconv.FormatInt(chatID, 10))
	if err != nil {
		return "", err
	}
	if entry == nil {
		return "", fmt.Errorf("unknown telegram chat id %d", chatID)
	}
	return c.ResolveChatWorkspaceHostPathByID(entry.ID, raw)
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

func (c *Config) migrateLegacyChatConfig() error {
	if c == nil || c.Store == nil {
		return nil
	}
	rootAny := c.Store.Get("telegram.chats", nil)
	root, ok := rootAny.(map[string]any)
	if !ok {
		return nil
	}

	for _, legacy := range legacyChatEntries(root) {
		providerChatID := strconv.FormatInt(legacy.ChatID, 10)
		existing := c.findProviderChatNoMigrate("telegram", providerChatID)
		if existing == nil {
			entry := &ChatConfigEntry{
				ID:                modeluuid.New(),
				ChatProviderType:  "telegram",
				ProviderChatID:    providerChatID,
				ProviderChatTitle: legacy.ChatTitle,
				Enabled:           legacy.Enabled,
			}
			if err := c.Store.PersistString(c.ChatKey(entry.ID, "chat_provider_type"), entry.ChatProviderType); err != nil {
				return err
			}
			if err := c.Store.PersistString(c.ChatKey(entry.ID, "provider_chat_id"), entry.ProviderChatID); err != nil {
				return err
			}
			if err := c.Store.PersistString(c.ChatKey(entry.ID, "provider_chat_title"), entry.ProviderChatTitle); err != nil {
				return err
			}
			if err := c.Store.PersistBool(c.ChatKey(entry.ID, "enabled"), entry.Enabled); err != nil {
				return err
			}
			if err := c.Store.PersistString(c.providerChatMapKey(entry.ChatProviderType, entry.ProviderChatID), entry.ID.String()); err != nil {
				return err
			}
			existing = entry
		}

		if raw := strings.TrimSpace(c.Store.GetString(c.legacyChatKey(legacy.ChatID, "workspace_host_path"), "")); raw != "" {
			if c.Store.Get(c.ChatKey(existing.ID, "workspace_host_path"), nil) == nil {
				if err := c.Store.PersistString(c.ChatKey(existing.ID, "workspace_host_path"), absOrEmpty(raw)); err != nil {
					return err
				}
			}
		}
		var commands []string
		if c.Store.GetStruct(c.legacyChatKey(legacy.ChatID, "hostbridge.allowed_commands"), &commands) && len(commands) > 0 {
			if c.Store.Get(c.ChatKey(existing.ID, "hostbridge.allowed_commands"), nil) == nil {
				if err := c.Store.PersistStruct(c.ChatKey(existing.ID, "hostbridge.allowed_commands"), commands); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func legacyChatEntries(root map[string]any) []struct {
	ChatID    int64
	ChatTitle string
	Enabled   bool
} {
	var out []struct {
		ChatID    int64
		ChatTitle string
		Enabled   bool
	}
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
			out = append(out, struct {
				ChatID    int64
				ChatTitle string
				Enabled   bool
			}{
				ChatID:    chatID,
				ChatTitle: stringFromAny(entryMap["chat_title"]),
				Enabled:   boolFromAny(entryMap["enabled"]),
			})
		}
	}
	if users, ok := root["users"].(map[string]any); ok {
		appendScope("users", users)
	}
	if groups, ok := root["groups"].(map[string]any); ok {
		appendScope("groups", groups)
	}
	return out
}

func (c *Config) legacyChatKey(chatID int64, key string) string {
	scope := "users"
	id := chatID
	if chatID < 0 {
		scope = "groups"
		id = -chatID
	}
	return fmt.Sprintf("telegram.chats.%s.%d.%s", scope, id, key)
}

func (c *Config) providerChatMapKey(providerType string, providerChatID string) string {
	return fmt.Sprintf("provider_chats.%x.%x", strings.TrimSpace(providerType), strings.TrimSpace(providerChatID))
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
