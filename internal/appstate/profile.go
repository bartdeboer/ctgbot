package appstate

import (
	"os"
	"path/filepath"
)

type ProfileConfig struct {
	cfg *Config
}

func (c *Config) Profile() ProfileConfig {
	return ProfileConfig{cfg: c}
}

func (p ProfileConfig) Root() string {
	if p.cfg == nil {
		return ""
	}
	return p.cfg.RootDir()
}

func (p ProfileConfig) DBPath() string {
	if p.Root() == "" {
		return ""
	}
	return filepath.Join(p.Root(), "ctgbot.db")
}

func (p ProfileConfig) TLSRoot() string {
	if p.Root() == "" {
		return ""
	}
	return filepath.Join(p.Root(), "tls")
}

func (p ProfileConfig) EnsurePaths() error {
	for _, dir := range []string{p.Root(), p.cfg.ChatsRoot()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return p.cfg.migrateLegacyLocalLayout()
}
