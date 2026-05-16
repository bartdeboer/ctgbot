package semantic

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const ComponentConfigFilename = "component.json"

const (
	DefaultBatchSize       = 40
	DefaultLimit           = 10
	DefaultMaxMessages     = 200
	DefaultMaxOutputTokens = 2048
	DefaultMinScore        = 0.4
)

type ComponentConfig struct {
	Completion      string  `json:"completion"`
	Model           string  `json:"model,omitempty"`
	BatchSize       int     `json:"batch_size,omitempty"`
	Limit           int     `json:"limit,omitempty"`
	MaxMessages     int     `json:"max_messages,omitempty"`
	MaxOutputTokens int     `json:"max_output_tokens,omitempty"`
	MinScore        float64 `json:"min_score,omitempty"`
	KeepWarmFor     string  `json:"keep_warm_for,omitempty"`
}

func loadComponentConfig(home string) (ComponentConfig, error) {
	var config ComponentConfig
	body, err := os.ReadFile(filepath.Join(home, ComponentConfigFilename))
	if err != nil {
		if os.IsNotExist(err) {
			return config.withDefaults(), nil
		}
		return ComponentConfig{}, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return config.withDefaults(), nil
	}
	if err := json.Unmarshal(body, &config); err != nil {
		return ComponentConfig{}, err
	}
	return config.withDefaults(), nil
}

func (c ComponentConfig) withDefaults() ComponentConfig {
	c.Completion = strings.TrimSpace(c.Completion)
	c.Model = strings.TrimSpace(c.Model)
	if c.BatchSize <= 0 {
		c.BatchSize = DefaultBatchSize
	}
	if c.Limit <= 0 {
		c.Limit = DefaultLimit
	}
	if c.MaxMessages < 0 {
		c.MaxMessages = 0
	}
	if c.MaxMessages == 0 {
		c.MaxMessages = DefaultMaxMessages
	}
	if c.MaxOutputTokens <= 0 {
		c.MaxOutputTokens = DefaultMaxOutputTokens
	}
	if c.MinScore <= 0 {
		c.MinScore = DefaultMinScore
	}
	return c
}
