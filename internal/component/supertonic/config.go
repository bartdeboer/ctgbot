package supertonic

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

const (
	Type                    = "supertonic"
	ComponentConfigFilename = "component.json"
	DefaultImage            = "ctgbot-supertonic:latest"
	DefaultDockerfile       = "supertonic.Dockerfile"
)

type ComponentConfig struct {
	ModelRegistry string `json:"model_registry,omitempty"`
	ModelStore    string `json:"model_store,omitempty"`
	DefaultModel  string `json:"default_model,omitempty"`
	DefaultVoice  string `json:"default_voice,omitempty"`
	Language      string `json:"language,omitempty"`
	PythonCommand string `json:"python_command,omitempty"`
	MaxConcurrent int    `json:"max_concurrent,omitempty"`
}

func loadRuntimeConfig(profilePath string) (runtimepkg.BindConfig, error) {
	config, err := runtimepkg.LoadBindConfig(profilePath)
	if err != nil {
		return runtimepkg.BindConfig{}, err
	}
	config.Image = firstNonEmpty(config.Image, DefaultImage)
	config.IdleTimeout = firstNonEmpty(config.IdleTimeout, "30s")
	if len(config.Cmd) == 0 {
		config.Entrypoint = firstNonEmpty(config.Entrypoint, "tail")
		config.Cmd = []string{"-f", "/dev/null"}
	}
	return config.Clean(), nil
}

func loadComponentConfig(profilePath string) (ComponentConfig, error) {
	var config ComponentConfig
	path := filepath.Join(strings.TrimSpace(profilePath), ComponentConfigFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return config.withDefaults(), nil
		}
		return ComponentConfig{}, fmt.Errorf("read supertonic component config %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return config.withDefaults(), nil
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return ComponentConfig{}, fmt.Errorf("read supertonic component config %s: %w", path, err)
	}
	return config.withDefaults(), nil
}

func (c ComponentConfig) withDefaults() ComponentConfig {
	c.ModelRegistry = firstNonEmpty(c.ModelRegistry, c.ModelStore, "model")
	c.ModelStore = strings.TrimSpace(c.ModelStore)
	c.DefaultModel = strings.TrimSpace(c.DefaultModel)
	c.DefaultVoice = firstNonEmpty(c.DefaultVoice, "F5")
	c.Language = firstNonEmpty(c.Language, "en")
	c.PythonCommand = firstNonEmpty(c.PythonCommand, "python")
	if c.MaxConcurrent == 0 {
		c.MaxConcurrent = 1
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
