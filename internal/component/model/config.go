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

func loadComponentConfig(profilePath string) (ComponentConfig, error) {
	var config ComponentConfig
	path := filepath.Join(strings.TrimSpace(profilePath), ComponentConfigFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return config.withDefaults(profilePath), nil
		}
		return ComponentConfig{}, fmt.Errorf("read model component config %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return config.withDefaults(profilePath), nil
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return ComponentConfig{}, fmt.Errorf("read model component config %s: %w", path, err)
	}
	return config.withDefaults(profilePath), nil
}

func (c ComponentConfig) withDefaults(profilePath string) ComponentConfig {
	c.ModelPath = cleanModelPath(c.ModelPath, profilePath)
	return c
}

func cleanModelPath(path string, profilePath string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return filepath.Join(strings.TrimSpace(profilePath), "models")
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(strings.TrimSpace(profilePath), path)
}
