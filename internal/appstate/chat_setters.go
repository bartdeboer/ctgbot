package appstate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
)

func (c ChatConfig) SetProviderType(providerType string) error {
	return c.persistString("chat_provider_type", strings.TrimSpace(providerType))
}

func (c ChatConfig) SetProviderChatID(providerChatID string) error {
	return c.persistString("provider_chat_id", strings.TrimSpace(providerChatID))
}

func (c ChatConfig) SetProviderChatTitle(title string) error {
	return c.persistString("provider_chat_title", strings.TrimSpace(title))
}

func (c ChatConfig) SetEnabled(enabled bool) error {
	return c.persistBool("enabled", enabled)
}

func (c ChatConfig) SetProcessToolsEnabled(enabled bool) error {
	return c.persistBool("process_tools", enabled)
}

func (c ChatConfig) SetInteractiveInterruptEnabled(enabled bool) error {
	return c.persistBool("interactive_interrupt", enabled)
}

func (c ChatConfig) SetContainerUserMode(raw string) error {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		mode = "default"
	}
	switch mode {
	case "default", "host", "root":
		return c.persistString("container_user_mode", mode)
	default:
		return fmt.Errorf("unsupported container user mode: %s", strings.TrimSpace(raw))
	}
}

func (c ChatConfig) SetWorkspaceHostPath(raw string) error {
	resolved, err := c.ResolveWorkspaceHostPath(raw)
	if err != nil {
		return err
	}
	return c.persistString("workspace_host_path", resolved)
}

func (c ChatConfig) ResolveWorkspaceHostPath(raw string) (string, error) {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return c.WorkspaceHostPath(), nil
	}
	return c.cfg.ResolveWorkspaceHostPath(candidate)
}

func (c ChatConfig) SetCodexProfileHostPath(raw string) error {
	resolved, err := c.ResolveCodexProfileHostPath(raw)
	if err != nil {
		return err
	}
	return c.persistString("codex_profile_host_path", resolved)
}

func (c ChatConfig) ResolveCodexProfileHostPath(raw string) (string, error) {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		candidate = c.CodexProfileHostPath()
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

func (c ChatConfig) SetSkills(skills []string) error {
	normalized, err := validateAndNormalizeSkillPaths(skills)
	if err != nil {
		return err
	}
	return c.persistStruct("skills", normalized)
}

func (c ChatConfig) AddSkill(skillDir string) error {
	return c.SetSkills(append(c.Skills(), skillDir))
}

func (c ChatConfig) RemoveSkill(skillDir string) error {
	target := absOrEmpty(skillDir)
	filtered := make([]string, 0, len(c.Skills()))
	for _, existing := range c.Skills() {
		if existing == target {
			continue
		}
		filtered = append(filtered, existing)
	}
	return c.persistStruct("skills", filtered)
}

func (h ChatHostbridgeConfig) SetAllowedCommand(name string, command hostbridgeserver.AllowedCommand) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("hostbridge allowed command name is empty")
	}
	normalized, ok := normalizeAllowedCommand(command)
	if !ok {
		return fmt.Errorf("hostbridge allowed command executable is empty")
	}
	commands := h.AllowedCommands()
	if commands == nil {
		commands = map[string]hostbridgeserver.AllowedCommand{}
	}
	commands[name] = normalized
	return h.chat.cfg.store.PersistStruct(h.key("allowed_commands"), commands)
}

func (h ChatHostbridgeConfig) ScaffoldAllowedCommand(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("hostbridge allowed command name is empty")
	}
	commands := map[string]hostbridgeserver.AllowedCommand{}
	_ = h.chat.cfg.structValue(h.key("allowed_commands"), &commands)
	commands[name] = hostbridgeserver.AllowedCommand{
		Args: []string{},
		Env:  map[string]string{},
	}
	return h.chat.cfg.store.PersistStruct(h.key("allowed_commands"), commands)
}

func (h ChatHostbridgeConfig) RemoveAllowedCommand(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("hostbridge allowed command name is empty")
	}
	commands := h.AllowedCommands()
	if len(commands) == 0 {
		return nil
	}
	for alias := range commands {
		if strings.EqualFold(alias, name) {
			delete(commands, alias)
		}
	}
	return h.chat.cfg.store.PersistStruct(h.key("allowed_commands"), commands)
}

func (c ChatConfig) persistString(key string, value string) error {
	if c.cfg == nil || c.cfg.store == nil {
		return fmt.Errorf("config store not available")
	}
	return c.cfg.store.PersistString(c.key(key), value)
}

func (c ChatConfig) persistBool(key string, value bool) error {
	if c.cfg == nil || c.cfg.store == nil {
		return fmt.Errorf("config store not available")
	}
	return c.cfg.store.PersistBool(c.key(key), value)
}

func (c ChatConfig) persistStruct(key string, value any) error {
	if c.cfg == nil || c.cfg.store == nil {
		return fmt.Errorf("config store not available")
	}
	return c.cfg.store.PersistStruct(c.key(key), value)
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

func normalizeAllowedCommand(spec hostbridgeserver.AllowedCommand) (hostbridgeserver.AllowedCommand, bool) {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Dir = strings.TrimSpace(spec.Dir)
	spec.Delay = strings.TrimSpace(spec.Delay)
	if spec.Name == "" {
		return hostbridgeserver.AllowedCommand{}, false
	}
	if len(spec.Args) == 0 {
		spec.Args = nil
	}
	if len(spec.Env) == 0 {
		spec.Env = nil
	}
	return spec, true
}
