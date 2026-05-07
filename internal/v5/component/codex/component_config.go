package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const ComponentConfigFilename = "component.json"

type ComponentConfig struct {
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
}

func loadComponentConfig(homePath string) (ComponentConfig, error) {
	path := filepath.Join(strings.TrimSpace(homePath), ComponentConfigFilename)
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
