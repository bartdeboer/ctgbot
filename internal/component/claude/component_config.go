package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const ComponentConfigFilename = "component.json"

const (
	DefaultImage             = "ctgbot-claude:latest"
	DefaultSessionTimeoutSec = 30 * 60
)

type ComponentConfig struct {
	Image             string `json:"image"`
	Dockerfile        string `json:"dockerfile"`
	Model             string `json:"model"`
	PermissionMode    string `json:"permission_mode"`
	SessionTimeoutSec int    `json:"session_timeout_sec"`
}

func loadComponentConfig(homePath string) (ComponentConfig, error) {
	path := filepath.Join(strings.TrimSpace(homePath), ComponentConfigFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ComponentConfig{}.withDefaults(), nil
		}
		return ComponentConfig{}, fmt.Errorf("read claude component config %s: %w", path, err)
	}
	var config ComponentConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return ComponentConfig{}, fmt.Errorf("read claude component config %s: %w", path, err)
	}
	return config.withDefaults(), nil
}

func (c ComponentConfig) withDefaults() ComponentConfig {
	c.Image = strings.TrimSpace(c.Image)
	c.Dockerfile = strings.TrimSpace(c.Dockerfile)
	c.Model = strings.TrimSpace(c.Model)
	c.PermissionMode = strings.TrimSpace(c.PermissionMode)
	if c.PermissionMode == "" {
		c.PermissionMode = "bypassPermissions"
	}
	if c.SessionTimeoutSec <= 0 {
		c.SessionTimeoutSec = DefaultSessionTimeoutSec
	}
	return c
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
