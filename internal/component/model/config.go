package model

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const ComponentConfigFilename = "component.json"

type ComponentConfig struct {
	ModelPath string `json:"model_path,omitempty"`
}

func loadComponentConfig(homePath string) (ComponentConfig, error) {
	var config ComponentConfig
	path := filepath.Join(strings.TrimSpace(homePath), ComponentConfigFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return config.withDefaults(homePath), nil
		}
		return ComponentConfig{}, fmt.Errorf("read model component config %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return config.withDefaults(homePath), nil
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return ComponentConfig{}, fmt.Errorf("read model component config %s: %w", path, err)
	}
	return config.withDefaults(homePath), nil
}

func (c ComponentConfig) withDefaults(homePath string) ComponentConfig {
	c.ModelPath = cleanModelPath(c.ModelPath, homePath)
	return c
}

func cleanModelPath(path string, homePath string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return filepath.Join(strings.TrimSpace(homePath), "models")
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(strings.TrimSpace(homePath), path)
}
