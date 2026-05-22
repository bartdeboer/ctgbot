package llamacppagent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/durationparse"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

const (
	Type                     = "llamacppagent"
	ComponentConfigFilename  = "component.json"
	DefaultImage             = "ctgbot-toolloop:latest"
	DefaultBaseImage         = "ctgbot-toolloop-base:latest"
	DefaultDevBaseImage      = "ctgbot-go-node-python-base:latest"
	DefaultDockerfile        = "toolloop.Dockerfile"
	DefaultBaseDockerfile    = "toolloop.base.Dockerfile"
	DefaultDevBaseDockerfile = "go-node-python.base.Dockerfile"
)

type ComponentConfig struct {
	Backend            string  `json:"backend,omitempty"`
	Model              string  `json:"model,omitempty"`
	BaseURL            string  `json:"base_url,omitempty"`
	APIKey             string  `json:"api_key,omitempty"`
	MaxIterations      int     `json:"max_iterations,omitempty"`
	MaxTokens          int     `json:"max_tokens,omitempty"`
	Temperature        float64 `json:"temperature,omitempty"`
	BackendIdleTimeout string  `json:"backend_idle_timeout,omitempty"`
	SystemPrompt       string  `json:"system_prompt,omitempty"`
}

func loadComponentConfig(homePath string) (ComponentConfig, error) {
	var config ComponentConfig
	path := filepath.Join(strings.TrimSpace(homePath), ComponentConfigFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return config.withDefaults(), nil
		}
		return ComponentConfig{}, fmt.Errorf("read llamacppagent config %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return ComponentConfig{}, fmt.Errorf("read llamacppagent config %s: %w", path, err)
	}
	return config.withDefaults(), nil
}

func (c ComponentConfig) withDefaults() ComponentConfig {
	c.Backend = firstNonEmpty(c.Backend, "llamacpp")
	c.Model = strings.TrimSpace(c.Model)
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	c.APIKey = strings.TrimSpace(c.APIKey)
	if c.MaxIterations <= 0 {
		c.MaxIterations = 20
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = 4096
	}
	if c.Temperature == 0 {
		c.Temperature = 0.2
	}
	c.BackendIdleTimeout = firstNonEmpty(c.BackendIdleTimeout, "30s")
	c.SystemPrompt = strings.TrimSpace(c.SystemPrompt)
	return c
}

func loadRuntimeConfig(homePath string) (runtimepkg.BindConfig, error) {
	config, err := runtimepkg.LoadBindConfig(homePath)
	if err != nil {
		return runtimepkg.BindConfig{}, err
	}
	config.Image = firstNonEmpty(config.Image, DefaultImage)
	config.Dockerfile = firstNonEmpty(config.Dockerfile, DefaultDockerfile)
	config.Cmd = append([]string{}, config.Cmd...)
	return config.Clean(), nil
}

func (c ComponentConfig) backendIdleTimeout() time.Duration {
	d, _ := durationparse.Parse(c.BackendIdleTimeout, time.Second)
	return d
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
