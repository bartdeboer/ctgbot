package appstate

import (
	"context"
	"fmt"
	"github.com/bartdeboer/ctgbot/internal/durationparse"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

func (c *Config) SetChatInteractiveInterruptEnabledByID(chatID modeluuid.UUID, enabled bool) error {
	if c == nil || c.Store == nil {
		return fmt.Errorf("config store not available")
	}
	if chatID.IsNull() {
		return fmt.Errorf("chat id is null")
	}
	return c.Store.PersistBool(c.ChatKey(chatID, "interactive_interrupt"), enabled)
}

func (c *Config) ChatInteractiveInterruptEnabledByID(chatID modeluuid.UUID) bool {
	if c == nil || c.Store == nil || chatID.IsNull() {
		return true
	}
	return c.Store.GetBool(c.ChatKey(chatID, "interactive_interrupt"), true)
}

func (c *Config) SetChatGPUsByID(chatID modeluuid.UUID, raw string) error {
	if c == nil || c.Store == nil {
		return fmt.Errorf("config store not available")
	}
	if chatID.IsNull() {
		return fmt.Errorf("chat id is null")
	}
	return c.Store.PersistString(c.ChatKey(chatID, "gpus"), strings.TrimSpace(raw))
}

func (c *Config) ChatGPUsByID(chatID modeluuid.UUID) string {
	if c == nil || c.Store == nil || chatID.IsNull() {
		return ""
	}
	return strings.TrimSpace(c.Store.GetString(c.ChatKey(chatID, "gpus"), ""))
}

func (c *Config) ChatWorkspaceHostPathByID(chatID modeluuid.UUID) string {
	if c == nil || c.Store == nil || chatID.IsNull() {
		return ""
	}
	return absOrEmpty(c.Store.GetString(c.ChatKey(chatID, "workspace_host_path"), ""))
}

func (c *Config) ChatSkillsByID(chatID modeluuid.UUID) []string {
	if c == nil || c.Store == nil || chatID.IsNull() {
		return nil
	}
	var out []string
	if !c.Store.GetStruct(c.ChatKey(chatID, "skills"), &out) {
		return nil
	}
	return normalizeSkillPaths(out)
}

func (c *Config) SetChatSkillsByID(chatID modeluuid.UUID, skills []string) error {
	if c == nil || c.Store == nil {
		return fmt.Errorf("config store not available")
	}
	if chatID.IsNull() {
		return fmt.Errorf("chat id is null")
	}
	normalized, err := validateAndNormalizeSkillPaths(skills)
	if err != nil {
		return err
	}
	return c.Store.PersistStruct(c.ChatKey(chatID, "skills"), normalized)
}

func (c *Config) AddChatSkillByID(chatID modeluuid.UUID, skillDir string) error {
	if c == nil || c.Store == nil {
		return fmt.Errorf("config store not available")
	}
	if chatID.IsNull() {
		return fmt.Errorf("chat id is null")
	}
	skills := append(c.ChatSkillsByID(chatID), skillDir)
	return c.SetChatSkillsByID(chatID, skills)
}

func (c *Config) RemoveChatSkillByID(chatID modeluuid.UUID, skillDir string) error {
	if c == nil || c.Store == nil {
		return fmt.Errorf("config store not available")
	}
	if chatID.IsNull() {
		return fmt.Errorf("chat id is null")
	}
	target := absOrEmpty(skillDir)
	filtered := make([]string, 0, len(c.ChatSkillsByID(chatID)))
	for _, existing := range c.ChatSkillsByID(chatID) {
		if existing == target {
			continue
		}
		filtered = append(filtered, existing)
	}
	return c.Store.PersistStruct(c.ChatKey(chatID, "skills"), filtered)
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
	spec.Delay = strings.TrimSpace(spec.Delay)
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

func validateAndNormalizeSkillPaths(skills []string) ([]string, error) {
	normalized := normalizeSkillPaths(skills)
	for _, skill := range normalized {
		if !filepath.IsAbs(skill) {
			return nil, fmt.Errorf("skill path must be absolute: %s", skill)
		}
	}
	return normalized, nil
}

func normalizeSkillPaths(skills []string) []string {
	if len(skills) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(skills))
	for _, skill := range skills {
		skill = absOrEmpty(skill)
		if skill == "" {
			continue
		}
		if _, ok := seen[skill]; ok {
			continue
		}
		seen[skill] = struct{}{}
		out = append(out, skill)
	}
	sort.Strings(out)
	return out
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
