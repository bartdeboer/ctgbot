package appstate

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (c *Config) Codex() CodexConfig {
	return CodexConfig{cfg: c}
}

type CodexConfig struct {
	cfg *Config
}

func (c CodexConfig) Model() string {
	return c.cfg.string("codex.model", "")
}

func (c CodexConfig) SetModel(model string) error {
	return c.cfg.persistString("codex.model", strings.TrimSpace(model))
}

// SessionTimeout adds no wall-clock deadline unless explicitly configured.
// Keep the legacy storage key and bare-number unit (minutes).
func (c CodexConfig) SessionTimeout() (time.Duration, error) {
	return parseCodexSessionTimeout(c.cfg.string("session.timeout_min", ""))
}

func (c CodexConfig) SetSessionTimeout(raw string) error {
	raw = strings.TrimSpace(raw)
	if _, err := parseCodexSessionTimeout(raw); err != nil {
		return err
	}
	return c.cfg.persistString("session.timeout_min", raw)
}

func parseCodexSessionTimeout(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	// ParseDuration checks overflow; multiplying a legacy integer by time.Minute
	// directly can wrap into a valid-looking (or negative) duration.
	if _, err := strconv.ParseInt(raw, 10, 64); err == nil {
		raw += "m"
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout < 0 {
		return 0, fmt.Errorf("invalid codex.session-timeout %q: use zero (no deadline) or a positive duration; bare numbers are minutes", raw)
	}
	return timeout, nil
}

func (c CodexConfig) ProfileHostPath() string {
	if raw := c.profileHostPathOverride(); raw != "" {
		return raw
	}
	for _, root := range c.ProfileCandidates() {
		if fileExistsAndNonEmpty(filepath.Join(root, "auth.json")) {
			return root
		}
	}
	return c.LocalProfileRoot()
}

func (c CodexConfig) SetProfileHostPath(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return c.cfg.persistString("codex.profile_host_path", "")
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	return c.cfg.persistString("codex.profile_host_path", abs)
}

func (c CodexConfig) CLIProfileRoot() string { return c.ProfileHostPath() }
func (c CodexConfig) LocalProfileRoot() string {
	if c.cfg == nil {
		return ""
	}
	return filepath.Join(c.cfg.RootDir(), ".codex")
}
func (c CodexConfig) ManagedProfileRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ctgbot", ".codex")
}
func (c CodexConfig) HostRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

func (c CodexConfig) EnsureCLIProfile() error {
	root := c.CLIProfileRoot()
	if root == "" {
		return fmt.Errorf("codex cli profile root is empty")
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	return c.importAuthIfNeeded()
}
func (c CodexConfig) AuthPath() string { return filepath.Join(c.CLIProfileRoot(), "auth.json") }
func (c CodexConfig) AuthSearchPaths() []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	for _, root := range c.ProfileCandidates() {
		if root == "" {
			continue
		}
		path := filepath.Join(root, "auth.json")
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}
func (c CodexConfig) ProfileCandidates() []string {
	return []string{c.LocalProfileRoot(), c.ManagedProfileRoot(), c.HostRoot()}
}
func (c CodexConfig) importAuthIfNeeded() error {
	dst := c.AuthPath()
	if fileExistsAndNonEmpty(dst) {
		return nil
	}
	for _, src := range c.AuthSearchPaths() {
		if !fileExistsAndNonEmpty(src) {
			continue
		}
		return copyFile(src, dst)
	}
	return nil
}
func (c CodexConfig) profileHostPathOverride() string {
	if raw := absOrEmpty(c.cfg.string("codex.profile_host_path", "")); raw != "" {
		return raw
	}
	return c.legacyProfileHostPathOverride()
}
