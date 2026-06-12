package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const ComponentConfigFilename = "component.json"
const DefaultSandboxMode = "danger-full-access"

type ComponentConfig struct {
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
	SandboxMode     string `json:"sandbox_mode"`
}

func loadComponentConfig(profilePath string) (ComponentConfig, error) {
	path := filepath.Join(strings.TrimSpace(profilePath), ComponentConfigFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ComponentConfig{}, nil
		}
		return ComponentConfig{}, fmt.Errorf("read codex component config %s: %w", path, err)
	}
	var config ComponentConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return ComponentConfig{}, fmt.Errorf("read codex component config %s: %w", path, err)
	}
	return config.withDefaults(), nil
}

func (c ComponentConfig) withDefaults() ComponentConfig {
	c.Model = strings.TrimSpace(c.Model)
	c.ReasoningEffort = strings.TrimSpace(c.ReasoningEffort)
	c.SandboxMode = strings.TrimSpace(c.SandboxMode)
	if c.SandboxMode == "" {
		c.SandboxMode = DefaultSandboxMode
	}
	return c
}
