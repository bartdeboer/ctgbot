package appstate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	hostbridgev2server "github.com/bartdeboer/ctgbot/internal/hostbridgev2/server"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func (c *Config) FindChatByID(chatID modeluuid.UUID) (*ChatConfigEntry, error) {
	return c.findChatByIDNoMigrate(chatID), nil
}

func (c *Config) FindProviderChat(providerType string, providerChatID string) (*ChatConfigEntry, error) {
	return c.findProviderChatNoMigrate(providerType, providerChatID), nil
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

func (c *Config) ChatEnabledByID(chatID modeluuid.UUID) bool {
	if c == nil || c.Store == nil || chatID.IsNull() {
		return false
	}
	return c.Store.GetBool(c.ChatKey(chatID, "enabled"), false)
}

func (c *Config) ChatProcessToolsEnabledByID(chatID modeluuid.UUID) bool {
	if c == nil || c.Store == nil || chatID.IsNull() {
		return false
	}
	return c.Store.GetBool(c.ChatKey(chatID, "process_tools"), false)
}

func (c *Config) ChatInteractiveInterruptEnabledByID(chatID modeluuid.UUID) bool {
	if c == nil || c.Store == nil || chatID.IsNull() {
		return true
	}
	return c.Store.GetBool(c.ChatKey(chatID, "interactive_interrupt"), true)
}

func (c *Config) ChatGPUsByID(chatID modeluuid.UUID) string {
	if c == nil || c.Store == nil || chatID.IsNull() {
		return ""
	}
	return strings.TrimSpace(c.Store.GetString(c.ChatKey(chatID, "gpus"), ""))
}

func (c *Config) ChatWorkspaceHostPathByID(chatID modeluuid.UUID) string {
	if c == nil || chatID.IsNull() {
		return ""
	}
	if raw := c.chatWorkspaceHostPathOverrideByID(chatID); raw != "" {
		return raw
	}
	if raw := c.DockerDefaultWorkspaceHostPath(); raw != "" {
		return raw
	}
	return c.DefaultChatWorkspaceDirByID(chatID)
}

func (c *Config) ChatCodexProfileHostPathByID(chatID modeluuid.UUID) string {
	if c == nil || chatID.IsNull() {
		return ""
	}
	if raw := c.chatCodexProfileHostPathOverrideByID(chatID); raw != "" {
		return raw
	}
	if raw := c.codexProfileHostPathOverride(); raw != "" {
		return raw
	}
	return c.DefaultChatCodexProfileDirByID(chatID)
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

func (c *Config) ChatHostbridgeAllowedCommandsByID(chatID modeluuid.UUID) map[string]hostbridgev2server.AllowedCommand {
	if c == nil || c.Store == nil || chatID.IsNull() {
		return nil
	}
	var out map[string]hostbridgev2server.AllowedCommand
	if c.Store.GetStruct(c.ChatKey(chatID, "hostbridge.allowed_commands"), &out) {
		return normalizeAllowedCommands(out)
	}

	var legacy []string
	if !c.Store.GetStruct(c.ChatKey(chatID, "hostbridge.allowed_commands"), &legacy) {
		return nil
	}
	return hostbridgev2server.AllowedCommandsFromSpecs(legacy)
}

func (c *Config) ResolveChatWorkspaceHostPathByID(chatID modeluuid.UUID, raw string) (string, error) {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return c.ChatWorkspaceHostPathByID(chatID), nil
	}
	return c.ResolveWorkspaceHostPath(candidate)
}

func (c *Config) ResolveChatCodexProfileHostPathByID(chatID modeluuid.UUID, raw string) (string, error) {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		candidate = c.ChatCodexProfileHostPathByID(chatID)
	}
	if candidate == "" {
		return "", fmt.Errorf("missing codex profile host path")
	}
	abs := absOrEmpty(candidate)
	if abs == "" {
		return "", fmt.Errorf("missing codex profile host path")
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", err
	}
	return abs, nil
}

func (c *Config) KnownChats() []ChatConfigEntry {
	return c.knownChatsNoMigrate()
}

func (c *Config) ChatKey(chatID modeluuid.UUID, key string) string {
	base := fmt.Sprintf(`chats["%s"]`, chatID.String())
	if strings.TrimSpace(key) == "" {
		return base
	}
	return base + "." + key
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

func (c *Config) SetChatProcessToolsEnabledByID(chatID modeluuid.UUID, enabled bool) error {
	if c == nil || c.Store == nil {
		return fmt.Errorf("config store not available")
	}
	if chatID.IsNull() {
		return fmt.Errorf("chat id is null")
	}
	return c.Store.PersistBool(c.ChatKey(chatID, "process_tools"), enabled)
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

func (c *Config) SetChatGPUsByID(chatID modeluuid.UUID, raw string) error {
	if c == nil || c.Store == nil {
		return fmt.Errorf("config store not available")
	}
	if chatID.IsNull() {
		return fmt.Errorf("chat id is null")
	}
	return c.Store.PersistString(c.ChatKey(chatID, "gpus"), strings.TrimSpace(raw))
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

func (c *Config) SetChatHostbridgeAllowedCommandByID(chatID modeluuid.UUID, name string, command hostbridgev2server.AllowedCommand) error {
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
		commands = map[string]hostbridgev2server.AllowedCommand{}
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

func (c *Config) SetChatCodexProfileHostPathByID(chatID modeluuid.UUID, raw string) error {
	if c == nil || c.Store == nil {
		return fmt.Errorf("config store not available")
	}
	if chatID.IsNull() {
		return fmt.Errorf("chat id is null")
	}
	resolved, err := c.ResolveChatCodexProfileHostPathByID(chatID, raw)
	if err != nil {
		return err
	}
	return c.Store.PersistString(c.ChatKey(chatID, "codex_profile_host_path"), resolved)
}

func (c *Config) chatWorkspaceHostPathOverrideByID(chatID modeluuid.UUID) string {
	if c == nil || c.Store == nil || chatID.IsNull() {
		return ""
	}
	return absOrEmpty(c.Store.GetString(c.ChatKey(chatID, "workspace_host_path"), ""))
}

func (c *Config) chatCodexProfileHostPathOverrideByID(chatID modeluuid.UUID) string {
	if c == nil || c.Store == nil || chatID.IsNull() {
		return ""
	}
	return absOrEmpty(c.Store.GetString(c.ChatKey(chatID, "codex_profile_host_path"), ""))
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

func normalizeAllowedCommands(raw map[string]hostbridgev2server.AllowedCommand) map[string]hostbridgev2server.AllowedCommand {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]hostbridgev2server.AllowedCommand, len(raw))
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

func hostbridgeNormalizeAllowedCommand(spec hostbridgev2server.AllowedCommand) (hostbridgev2server.AllowedCommand, bool) {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Dir = strings.TrimSpace(spec.Dir)
	spec.Delay = strings.TrimSpace(spec.Delay)
	if spec.Name == "" {
		return hostbridgev2server.AllowedCommand{}, false
	}
	if len(spec.Args) == 0 {
		spec.Args = nil
	}
	if len(spec.Env) == 0 {
		spec.Env = nil
	}
	return spec, true
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
