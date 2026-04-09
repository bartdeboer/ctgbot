package appconfig

import (
	"context"
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

	"github.com/bartdeboer/ctgbot/internal/hostbridge"
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
	ID                modeluuid.UUID
	ProviderType      string
	ProviderChatID    string
	ProviderChatTitle string
	Enabled           bool
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

func (c *Config) ChatClientIdentity(chatID modeluuid.UUID) string {
	if chatID.IsNull() {
		return ""
	}
	return chatClientNamePrefix + chatID.String()
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

func (c *Config) ParseChatClientIdentity(name string) (chatID modeluuid.UUID, ok bool) {
	raw := strings.TrimPrefix(strings.TrimSpace(name), chatClientNamePrefix)
	if raw == "" || raw == name {
		return modeluuid.Nil, false
	}
	chatID, err := modeluuid.Parse(raw)
	if err != nil {
		return modeluuid.Nil, false
	}
	return chatID, true
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

func (c *Config) HostGitIdentity(ctx context.Context) GitIdentity {
	return GitIdentity{
		Name:  strings.TrimSpace(readGitConfig(ctx, "user.name")),
		Email: strings.TrimSpace(readGitConfig(ctx, "user.email")),
	}
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
		ProviderType:      strings.TrimSpace(c.Store.GetString(c.ChatKey(chatID, "chat_provider_type"), c.Store.GetString(c.ChatKey(chatID, "type"), ""))),
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
	return c.findProviderChatNoMigrate(providerType, providerChatID), nil
}

func (c *Config) findProviderChatNoMigrate(providerType string, providerChatID string) *ChatConfigEntry {
	providerType = strings.TrimSpace(providerType)
	providerChatID = strings.TrimSpace(providerChatID)
	if c == nil || c.Store == nil || providerType == "" || providerChatID == "" {
		return nil
	}

	for _, chat := range c.knownChatsNoMigrate() {
		if chat.ProviderType == providerType && chat.ProviderChatID == providerChatID {
			entry := chat
			return &entry
		}
	}
	return nil
}

func (c *Config) EnsureProviderChat(providerType string, providerChatID string, title string) (*ChatConfigEntry, error) {
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
		ProviderType:      providerType,
		ProviderChatID:    providerChatID,
		ProviderChatTitle: title,
		Enabled:           false,
	}

	if err := c.Store.PersistString(c.ChatKey(entry.ID, "chat_provider_type"), entry.ProviderType); err != nil {
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
	return entry, nil
}

func (c *Config) SetChatEnabledByID(chatID modeluuid.UUID, enabled bool) error {
	if c == nil || c.Store == nil {
		return fmt.Errorf("config store not available")
	}
	if chatID.IsNull() {
		return fmt.Errorf("chat id is null")
	}
	return c.Store.PersistBool(c.ChatKey(chatID, "enabled"), enabled)
}

func (c *Config) ChatEnabledByID(chatID modeluuid.UUID) bool {
	if c == nil || c.Store == nil || chatID.IsNull() {
		return false
	}
	return c.Store.GetBool(c.ChatKey(chatID, "enabled"), false)
}

func (c *Config) SetChatProcessToolsEnabledByID(chatID modeluuid.UUID, enabled bool) error {
	if c == nil || c.Store == nil {
		return fmt.Errorf("config store not available")
	}
	if chatID.IsNull() {
		return fmt.Errorf("chat id is null")
	}
	return c.Store.PersistBool(c.ChatKey(chatID, "process_tools"), enabled)
}

func (c *Config) ChatProcessToolsEnabledByID(chatID modeluuid.UUID) bool {
	if c == nil || c.Store == nil || chatID.IsNull() {
		return false
	}
	return c.Store.GetBool(c.ChatKey(chatID, "process_tools"), false)
}

func (c *Config) ChatWorkspaceHostPathByID(chatID modeluuid.UUID) string {
	if c == nil || c.Store == nil || chatID.IsNull() {
		return ""
	}
	return absOrEmpty(c.Store.GetString(c.ChatKey(chatID, "workspace_host_path"), ""))
}

func (c *Config) SetChatWorkspaceHostPathByID(chatID modeluuid.UUID, raw string) error {
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

func (c *Config) ChatHostbridgeAllowedCommandsByID(chatID modeluuid.UUID) map[string]hostbridge.AllowedCommand {
	if c == nil || c.Store == nil || chatID.IsNull() {
		return nil
	}
	var out map[string]hostbridge.AllowedCommand
	if c.Store.GetStruct(c.ChatKey(chatID, "hostbridge.allowed_commands"), &out) {
		return normalizeAllowedCommands(out)
	}

	var legacy []string
	if !c.Store.GetStruct(c.ChatKey(chatID, "hostbridge.allowed_commands"), &legacy) {
		return nil
	}
	return hostbridge.AllowedCommandsFromSpecs(legacy)
}

func (c *Config) ChatHostbridgeAllowedCommandSpecsByID(chatID modeluuid.UUID) []string {
	allowed := c.ChatHostbridgeAllowedCommandsByID(chatID)
	if len(allowed) == 0 {
		return nil
	}
	names := hostbridge.AllowedCommandNames(allowed)
	out := make([]string, 0, len(names))
	for _, name := range names {
		spec := allowed[name]
		out = append(out, spec.Name)
	}
	return out
}

func (c *Config) SetChatHostbridgeAllowedCommandByID(chatID modeluuid.UUID, name string, command hostbridge.AllowedCommand) error {
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
	normalized, ok := hostbridgeNormalizeAllowedCommand(command)
	if !ok {
		return fmt.Errorf("hostbridge allowed command executable is empty")
	}

	commands := c.ChatHostbridgeAllowedCommandsByID(chatID)
	if commands == nil {
		commands = map[string]hostbridge.AllowedCommand{}
	}
	commands[name] = normalized
	return c.Store.PersistStruct(c.ChatKey(chatID, "hostbridge.allowed_commands"), commands)
}

func (c *Config) RemoveChatHostbridgeAllowedCommandByID(chatID modeluuid.UUID, name string) error {
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

	commands := c.ChatHostbridgeAllowedCommandsByID(chatID)
	if len(commands) == 0 {
		return nil
	}
	for alias := range commands {
		if strings.EqualFold(alias, name) {
			delete(commands, alias)
		}
	}
	return c.Store.PersistStruct(c.ChatKey(chatID, "hostbridge.allowed_commands"), commands)
}

func (c *Config) ResolveChatWorkspaceHostPathByID(chatID modeluuid.UUID, raw string) (string, error) {
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
			ProviderType:      firstNonEmptyString(entryMap["chat_provider_type"], entryMap["type"]),
			ProviderChatID:    stringFromAny(entryMap["provider_chat_id"]),
			ProviderChatTitle: stringFromAny(entryMap["provider_chat_title"]),
			Enabled:           boolFromAny(entryMap["enabled"]),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].ProviderType != out[j].ProviderType {
			return out[i].ProviderType < out[j].ProviderType
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
	base := fmt.Sprintf(`chats["%s"]`, chatID.String())
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

func (c *Config) ChatProcessToolsEnabled(chatID int64) bool {
	entry, err := c.FindProviderChat("telegram", strconv.FormatInt(chatID, 10))
	if err != nil || entry == nil {
		return false
	}
	return c.ChatProcessToolsEnabledByID(entry.ID)
}

func (c *Config) SetChatProcessToolsEnabled(chatID int64, enabled bool) error {
	entry, err := c.EnsureProviderChat("telegram", strconv.FormatInt(chatID, 10), "")
	if err != nil {
		return err
	}
	return c.SetChatProcessToolsEnabledByID(entry.ID, enabled)
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

func (c *Config) ChatHostbridgeAllowedCommands(chatID int64) map[string]hostbridge.AllowedCommand {
	entry, err := c.FindProviderChat("telegram", strconv.FormatInt(chatID, 10))
	if err != nil || entry == nil {
		return nil
	}
	return c.ChatHostbridgeAllowedCommandsByID(entry.ID)
}

func (c *Config) SetChatHostbridgeAllowedCommand(chatID int64, name string, command hostbridge.AllowedCommand) error {
	entry, err := c.EnsureProviderChat("telegram", strconv.FormatInt(chatID, 10), "")
	if err != nil {
		return err
	}
	return c.SetChatHostbridgeAllowedCommandByID(entry.ID, name, command)
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

func normalizeAllowedCommands(raw map[string]hostbridge.AllowedCommand) map[string]hostbridge.AllowedCommand {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]hostbridge.AllowedCommand, len(raw))
	for name, spec := range raw {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if normalized, ok := hostbridgeNormalizeAllowedCommand(spec); ok {
			out[name] = normalized
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func hostbridgeNormalizeAllowedCommand(spec hostbridge.AllowedCommand) (hostbridge.AllowedCommand, bool) {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Dir = strings.TrimSpace(spec.Dir)
	if spec.Name == "" {
		return hostbridge.AllowedCommand{}, false
	}
	if len(spec.Args) == 0 {
		spec.Args = nil
	}
	if len(spec.Env) == 0 {
		spec.Env = nil
	}
	return spec, true
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
