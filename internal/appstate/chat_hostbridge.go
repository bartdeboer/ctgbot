package appstate

import (
	"fmt"
	"strings"

	hostbridgepolicy "github.com/bartdeboer/ctgbot/internal/hostbridgepolicy"
)

type ChatHostbridgeConfig struct {
	chat ChatConfig
}

func (h ChatHostbridgeConfig) AllowedCommands() map[string]hostbridgepolicy.AllowedCommand {
	var current map[string]hostbridgepolicy.AllowedCommand
	if h.chat.cfg.structValue(h.key("allowed_commands"), &current) {
		return normalizeAllowedCommands(current)
	}
	return nil
}

func (h ChatHostbridgeConfig) ConfiguredAllowedCommands() map[string]hostbridgepolicy.AllowedCommand {
	var current map[string]hostbridgepolicy.AllowedCommand
	if h.chat.cfg.structValue(h.key("allowed_commands"), &current) {
		return normalizeConfiguredAllowedCommands(current)
	}
	return nil
}

func (h ChatHostbridgeConfig) key(key string) string {
	return fmt.Sprintf(`chats["%s"].hostbridge.%s`, h.chat.chatID.String(), strings.TrimSpace(key))
}

func normalizeAllowedCommands(raw map[string]hostbridgepolicy.AllowedCommand) map[string]hostbridgepolicy.AllowedCommand {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]hostbridgepolicy.AllowedCommand, len(raw))
	for name, spec := range raw {
		name = strings.TrimSpace(name)
		spec.Name = strings.TrimSpace(spec.Name)
		spec.Dir = strings.TrimSpace(spec.Dir)
		spec.Delay = strings.TrimSpace(spec.Delay)
		if name == "" || spec.Name == "" {
			continue
		}
		if len(spec.Args) == 0 {
			spec.Args = nil
		}
		if len(spec.Env) == 0 {
			spec.Env = nil
		}
		out[name] = spec
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeConfiguredAllowedCommands(raw map[string]hostbridgepolicy.AllowedCommand) map[string]hostbridgepolicy.AllowedCommand {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]hostbridgepolicy.AllowedCommand, len(raw))
	for name, spec := range raw {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		spec.Name = strings.TrimSpace(spec.Name)
		spec.Dir = strings.TrimSpace(spec.Dir)
		spec.Delay = strings.TrimSpace(spec.Delay)
		if len(spec.Args) == 0 {
			spec.Args = nil
		}
		if len(spec.Env) == 0 {
			spec.Env = nil
		}
		out[name] = spec
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
