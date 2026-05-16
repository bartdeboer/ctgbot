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
	DefaultMaxOutputTokens = 2048
	DefaultMinScore        = 0.4
)

type ComponentConfig struct {
	Completion      string  `json:"completion"`
	BatchSize       int     `json:"batch_size,omitempty"`
	Limit           int     `json:"limit,omitempty"`
	MaxOutputTokens int     `json:"max_output_tokens,omitempty"`
	MinScore        float64 `json:"min_score,omitempty"`
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
	if c.BatchSize <= 0 {
		c.BatchSize = DefaultBatchSize
	}
	if c.Limit <= 0 {
		c.Limit = DefaultLimit
	}
	if c.MaxOutputTokens <= 0 {
		c.MaxOutputTokens = DefaultMaxOutputTokens
	}
	if c.MinScore <= 0 {
		c.MinScore = DefaultMinScore
	}
	return c
}
