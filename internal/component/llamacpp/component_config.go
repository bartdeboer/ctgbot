package llamacpp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

const (
	Type                    = "llamacpp"
	DefaultImage            = "ghcr.io/ggml-org/llama.cpp:server-cuda"
	ComponentConfigFilename = "component.json"
)

type ComponentConfig struct {
	ModelStore     string  `json:"model_store,omitempty"`
	DefaultModel   string  `json:"default_model,omitempty"`
	ModelPath      string  `json:"model_path"`
	MMProjPath     string  `json:"mmproj_path,omitempty"`
	HostPort       int     `json:"host_port"`
	ContextSize    int     `json:"ctx_size"`
	GPULayers      int     `json:"gpu_layers"`
	MaxTokens      int     `json:"max_tokens"`
	Temperature    float64 `json:"temperature"`
	KeepRunning    bool    `json:"keep_running"`
	StripReasoning bool    `json:"strip_reasoning"`
}

func loadRuntimeConfig(homePath string) (runtimepkg.BindConfig, error) {
	config, err := runtimepkg.LoadBindConfig(homePath)
	if err != nil {
		return runtimepkg.BindConfig{}, err
	}
	config.Image = firstNonEmpty(config.Image, DefaultImage)
	config.GPUs = firstNonEmpty(config.GPUs, "all")
	return config.Clean(), nil
}

func loadComponentConfig(homePath string, name string) (ComponentConfig, error) {
	_ = name
	var config ComponentConfig
	path := filepath.Join(strings.TrimSpace(homePath), ComponentConfigFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return config.withDefaults(), nil
		}
		return ComponentConfig{}, fmt.Errorf("read llamacpp component config %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return ComponentConfig{}, fmt.Errorf("read llamacpp component config %s: %w", path, err)
	}
	return config.withDefaults(), nil
}

func (c ComponentConfig) withDefaults() ComponentConfig {
	c.ModelStore = strings.TrimSpace(c.ModelStore)
	if c.ModelStore == "" {
		c.ModelStore = "model"
	}
	c.DefaultModel = strings.TrimSpace(c.DefaultModel)
	c.ModelPath = strings.TrimSpace(c.ModelPath)
	c.MMProjPath = strings.TrimSpace(c.MMProjPath)
	if c.HostPort == 0 {
		c.HostPort = 19080
	}
	if c.ContextSize == 0 {
		c.ContextSize = 8192
	}
	if c.GPULayers == 0 {
		c.GPULayers = 99
	}
	if c.MaxTokens == 0 {
		c.MaxTokens = 1024
	}
	if c.Temperature == 0 {
		c.Temperature = 0.2
	}
	c.StripReasoning = true
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
