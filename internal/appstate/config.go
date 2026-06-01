package appstate

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/durationparse"
	"github.com/bartdeboer/go-clistate"
)

type Config struct {
	root   string
	store  *clistate.Store
	global *clistate.Store
}

func New(root string, store *clistate.Store, globalStore ...*clistate.Store) *Config {
	var global *clistate.Store
	if len(globalStore) > 0 {
		global = globalStore[0]
	}
	return &Config{root: absOrEmpty(root), store: store, global: global}
}

func (c *Config) RootDir() string {
	if c == nil {
		return ""
	}
	return c.root
}

func (c *Config) ProjectDir() string {
	return c.Global().ProjectDir()
}

func (c *Config) ProjectRoot() string {
	if c == nil || c.root == "" {
		return ""
	}
	return filepath.Dir(c.root)
}

func (c *Config) DBPath() string {
	return c.Profile().DBPath()
}

func (c *Config) string(key string, fallback string) string {
	if c == nil || c.store == nil {
		return fallback
	}
	return strings.TrimSpace(c.store.GetString(key, fallback))
}

func (c *Config) bool(key string, fallback bool) bool {
	if c == nil || c.store == nil {
		return fallback
	}
	return c.store.GetBool(key, fallback)
}

func (c *Config) duration(key string, fallback int, unit time.Duration) time.Duration {
	raw := c.string(key, "")
	if raw == "" {
		return time.Duration(fallback) * unit
	}
	parsed, err := durationparse.Parse(raw, unit)
	if err != nil || parsed == 0 {
		return time.Duration(fallback) * unit
	}
	return parsed
}

func (c *Config) structValue(key string, out any) bool {
	if c == nil || c.store == nil {
		return false
	}
	return c.store.GetStruct(key, out)
}

func (c *Config) ResolveStruct(key string, out any) (fmt.Stringer, bool, error) {
	if c == nil || c.store == nil {
		return nil, false, errMissingConfigStore()
	}
	return c.store.ResolveStruct(key, out)
}

func (c *Config) PersistOverlayStruct(layerName string, key string, val any) error {
	if c == nil || c.store == nil {
		return errMissingConfigStore()
	}
	return c.store.PersistOverlayStruct(layerName, key, val)
}

func (c *Config) UnsetOverlay(layerName string, key string) error {
	if c == nil || c.store == nil {
		return errMissingConfigStore()
	}
	return c.store.UnsetOverlay(layerName, key)
}

func (c *Config) persistString(key string, value string) error {
	if c == nil || c.store == nil {
		return errMissingConfigStore()
	}
	return c.store.PersistString(key, value)
}

func absOrEmpty(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	return abs
}
