package appstate

import (
	"fmt"
	"strings"

	hostbridgepolicy "github.com/bartdeboer/ctgbot/internal/hostbridgepolicy"
)

type ChatHostbridgeConfig struct {
	chat ChatConfig
}

func (h ChatHostbridgeConfig) Aliases() map[string]hostbridgepolicy.Alias {
	var current map[string]hostbridgepolicy.Alias
	if h.chat.cfg.structValue(h.key("allowed_commands"), &current) {
		return normalizeAliases(current)
	}
	return nil
}

func (h ChatHostbridgeConfig) ConfiguredAliases() map[string]hostbridgepolicy.Alias {
	var current map[string]hostbridgepolicy.Alias
	if h.chat.cfg.structValue(h.key("allowed_commands"), &current) {
		return normalizeConfiguredAliases(current)
	}
	return nil
}

func (h ChatHostbridgeConfig) key(key string) string {
	return fmt.Sprintf(`chats["%s"].hostbridge.%s`, h.chat.chatID.String(), strings.TrimSpace(key))
}

func normalizeAliases(raw map[string]hostbridgepolicy.Alias) map[string]hostbridgepolicy.Alias {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]hostbridgepolicy.Alias, len(raw))
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

func normalizeConfiguredAliases(raw map[string]hostbridgepolicy.Alias) map[string]hostbridgepolicy.Alias {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]hostbridgepolicy.Alias, len(raw))
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
