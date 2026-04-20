package appstate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (c *Config) CodexSessionTimeout() time.Duration {
	return c.durationFromConfig("session.timeout_min", 10, time.Minute)
}

func (c *Config) CodexModel() string {
	if c == nil || c.Store == nil {
		return ""
	}
	return strings.TrimSpace(c.Store.GetString("codex.model", ""))
}

func (c *Config) CodexProfileHostPath() string {
	if c == nil {
		return ""
	}
	if raw := c.codexProfileHostPathOverride(); raw != "" {
		return raw
	}
	for _, root := range c.codexCLIHomeCandidates() {
		if fileExistsAndNonEmpty(filepath.Join(root, "auth.json")) {
			return root
		}
	}
	return c.LocalCodexCLIHomeRoot()
}

// CodexCLIHomeRoot is a legacy compatibility alias for the canonical
// codex profile host path used by Codex on the host.
func (c *Config) CodexCLIHomeRoot() string {
	return c.CodexProfileHostPath()
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

func (c *Config) CodexCLIHomeAuthPath() string {
	return filepath.Join(c.CodexCLIHomeRoot(), "auth.json")
}

func (c *Config) CodexAuthSearchPaths() []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	for _, root := range c.codexCLIHomeCandidates() {
		if root == "" {
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

func (c *Config) importAuthIfNeeded() error {
	dst := c.CodexCLIHomeAuthPath()
	if fileExistsAndNonEmpty(dst) {
		return nil
	}
	for _, src := range c.CodexAuthSearchPaths() {
		if !fileExistsAndNonEmpty(src) {
			continue
		}
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		out, err := os.Create(dst)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	}
	return nil
}

func (c *Config) codexCLIHomeCandidates() []string {
	return []string{
		c.LocalCodexCLIHomeRoot(),
		c.ManagedHomeCodexCLIHomeRoot(),
		c.HostCodexRoot(),
	}
}

func (c *Config) codexProfileHostPathOverride() string {
	if c == nil || c.Store == nil {
		return ""
	}
	for _, key := range []string{"codex.profile_host_path", "codex.cli_home_host_path", "codex.shared_home_host_path"} {
		if raw := absOrEmpty(c.Store.GetString(key, "")); raw != "" {
			return raw
		}
	}
	return ""
}
