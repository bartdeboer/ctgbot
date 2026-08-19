package hostbridgepolicy

import (
	"fmt"
	"path/filepath"
	"strings"
)

// NormalizeAlias returns an isolated, consistently normalized policy value.
// It does not require Name so configuration editors can retain incomplete
// scaffold entries.
func NormalizeAlias(spec Alias) Alias {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.ArgsPattern = strings.TrimSpace(spec.ArgsPattern)
	spec.Dir = strings.TrimSpace(spec.Dir)
	spec.Delay = strings.TrimSpace(spec.Delay)
	spec.SerializationKey = strings.ToLower(strings.TrimSpace(spec.SerializationKey))
	spec.Args = cloneStrings(spec.Args)
	spec.AllowedCWDs = normalizeCWDs(spec.AllowedCWDs)
	spec.Subcommands = normalizeSubcommands(spec.Subcommands)
	spec.Env = normalizeEnv(spec.Env)
	return spec
}

// ValidateAlias checks policy contradictions that do not depend on current
// host filesystem state. Directory existence remains an invocation-time fact.
func ValidateAlias(spec Alias) error {
	if spec.Name == "" {
		return fmt.Errorf("executable is empty")
	}
	if spec.Dir != "" && len(spec.AllowedCWDs) > 0 {
		return fmt.Errorf("dir cannot be combined with allowed_cwds")
	}
	for _, path := range spec.AllowedCWDs {
		if path == "" {
			return fmt.Errorf("allowed_cwds contains an empty path")
		}
		if !filepath.IsAbs(path) {
			return fmt.Errorf("allowed_cwds path must be absolute: %s", path)
		}
	}
	return nil
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string{}, values...)
}

func normalizeCWDs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	for index, value := range values {
		out[index] = strings.TrimSpace(value)
	}
	return out
}

func normalizeSubcommands(subcommands map[string]AliasSubcommand) map[string]AliasSubcommand {
	if len(subcommands) == 0 {
		return nil
	}
	out := make(map[string]AliasSubcommand, len(subcommands))
	for name, subcommand := range subcommands {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		subcommand.Args = cloneStrings(subcommand.Args)
		subcommand.ArgsPattern = strings.TrimSpace(subcommand.ArgsPattern)
		out[name] = subcommand
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeEnv(env map[string]string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for key, value := range env {
		key = strings.TrimSpace(key)
		if key == "" || strings.ContainsRune(key, '=') {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
