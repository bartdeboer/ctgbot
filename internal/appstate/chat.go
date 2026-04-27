package appstate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type ChatConfig struct {
	cfg    *Config
	chatID modeluuid.UUID
}

func (c *Config) Chat(chatID modeluuid.UUID) ChatConfig {
	return ChatConfig{cfg: c, chatID: chatID}
}
func (c ChatConfig) ID() modeluuid.UUID {
	return c.chatID
}

func (c ChatConfig) ProviderType() string {
	return c.cfg.string(c.key("chat_provider_type"), c.cfg.string(c.key("type"), ""))
}

func (c ChatConfig) ProviderChatID() string {
	return c.cfg.string(c.key("provider_chat_id"), "")
}

func (c ChatConfig) ProviderChatTitle() string {
	return c.cfg.string(c.key("provider_chat_title"), "")
}

func (c ChatConfig) Enabled() bool {
	return c.cfg.bool(c.key("enabled"), false)
}

func (c ChatConfig) ProcessToolsEnabled() bool {
	return c.cfg.bool(c.key("process_tools"), false)
}

func (c ChatConfig) InteractiveInterruptEnabled() bool {
	return c.cfg.bool(c.key("interactive_interrupt"), true)
}

func (c ChatConfig) GPUs() string {
	return c.cfg.string(c.key("gpus"), "")
}

func (c ChatConfig) ContainerUserMode() string {
	mode := normalizeContainerUserMode(c.cfg.string(c.key("container_user_mode"), "default"))
	if mode == "" {
		return "default"
	}
	return mode
}

func (c ChatConfig) WorkspaceHostPath() string {
	if raw := absOrEmpty(c.cfg.string(c.key("workspace_host_path"), "")); raw != "" {
		return raw
	}
	if raw := c.cfg.Docker().DefaultWorkspaceHostPath(); raw != "" {
		return raw
	}
	return c.Profile().WorkspaceDir()
}

func (c ChatConfig) CodexProfileHostPath() string {
	if raw := absOrEmpty(c.cfg.string(c.key("codex_profile_host_path"), "")); raw != "" {
		return raw
	}
	if raw := c.cfg.Codex().profileHostPathOverride(); raw != "" {
		return raw
	}
	return c.Profile().CodexProfileDir()
}

func (c ChatConfig) Skills() []string {
	var out []string
	if !c.cfg.structValue(c.key("skills"), &out) {
		return nil
	}
	return normalizeSkillPaths(out)
}

func (c ChatConfig) Hostbridge() ChatHostbridgeConfig {
	return ChatHostbridgeConfig{chat: c}
}

func (c ChatConfig) RuntimeRoot() string {
	return c.Profile().Root()
}

func (c ChatConfig) DefaultCodexProfileDir() string {
	return c.Profile().CodexProfileDir()
}

func (c ChatConfig) DefaultWorkspaceDir() string {
	return c.Profile().WorkspaceDir()
}

func (c ChatConfig) DefaultLogDir() string {
	return c.Profile().LogDir()
}

func (c ChatConfig) DefaultTLSDir() string {
	return c.Profile().TLSDir()
}

func (c ChatConfig) ThreadsRoot() string {
	return c.Profile().ThreadsRoot()
}

func (c ChatConfig) ClientIdentity() string {
	if c.chatID.IsNull() {
		return ""
	}
	return "ctgbot-chat-" + c.chatID.String()
}

func (c ChatConfig) key(key string) string {
	return fmt.Sprintf(`chats["%s"].%s`, c.chatID.String(), strings.TrimSpace(key))
}

func normalizeContainerUserMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "", "default", "host", "root":
		return mode
	default:
		return "default"
	}
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

type ChatConfigEntry struct {
	ID                          modeluuid.UUID
	ProviderType                string
	ProviderChatID              string
	ProviderChatTitle           string
	Enabled                     bool
	InteractiveInterruptEnabled bool
}

func (c *Config) FindChatByID(chatID modeluuid.UUID) (*ChatConfigEntry, error) {
	return c.findChatByID(chatID), nil
}

func (c *Config) FindProviderChat(providerType string, providerChatID string) (*ChatConfigEntry, error) {
	providerType = strings.TrimSpace(providerType)
	providerChatID = strings.TrimSpace(providerChatID)
	if providerType == "" || providerChatID == "" {
		return nil, nil
	}
	for _, chat := range c.KnownChats() {
		if chat.ProviderType == providerType && chat.ProviderChatID == providerChatID {
			entry := chat
			return &entry, nil
		}
	}
	return nil, nil
}

func (c *Config) EnsureProviderChat(providerType string, providerChatID string, title string) (*ChatConfigEntry, error) {
	if c == nil || c.store == nil {
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
			if err := c.Chat(existing.ID).SetProviderChatTitle(title); err != nil {
				return nil, err
			}
			existing.ProviderChatTitle = title
		}
		return existing, nil
	}
	entry := &ChatConfigEntry{ID: modeluuid.New(), ProviderType: providerType, ProviderChatID: providerChatID, ProviderChatTitle: title}
	chat := c.Chat(entry.ID)
	if err := chat.SetProviderType(providerType); err != nil {
		return nil, err
	}
	if err := chat.SetProviderChatID(providerChatID); err != nil {
		return nil, err
	}
	if err := chat.SetProviderChatTitle(title); err != nil {
		return nil, err
	}
	if err := chat.SetEnabled(false); err != nil {
		return nil, err
	}
	return entry, nil
}

func (c *Config) KnownChats() []ChatConfigEntry {
	if c == nil || c.store == nil {
		return nil
	}
	rootAny := c.store.Get("chats", nil)
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

func (c *Config) findChatByID(chatID modeluuid.UUID) *ChatConfigEntry {
	if c == nil || c.store == nil || chatID.IsNull() {
		return nil
	}
	chat := c.Chat(chatID)
	if !chat.Exists() {
		return nil
	}
	return &ChatConfigEntry{
		ID:                chatID,
		ProviderType:      chat.ProviderType(),
		ProviderChatID:    chat.ProviderChatID(),
		ProviderChatTitle: chat.ProviderChatTitle(),
		Enabled:           chat.Enabled(),
	}
}

func (c ChatConfig) Exists() bool {
	if c.cfg == nil || c.cfg.store == nil || c.chatID.IsNull() {
		return false
	}
	for _, key := range []string{"chat_provider_type", "type", "provider_chat_id", "provider_chat_title", "enabled"} {
		if c.cfg.store.Get(c.key(key), nil) != nil {
			return true
		}
	}
	return false
}

func (c ChatConfig) RuntimeName() string {
	if c.chatID.IsNull() {
		return ""
	}
	return c.chatID.String()
}
