package semantic

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const ComponentConfigFilename = "component.json"

const (
	DefaultBatchSize          = 40
	DefaultLimit              = 10
	DefaultExcerptSize        = 240
	MaxExcerptSize            = 1000
	MaxSearchResults          = 50
	DefaultFullSearchResults  = 3
	MaxFullSearchResults      = 5
	DefaultMaxMessages        = 200
	DefaultMaxOutputTokens    = 2048
	DefaultMinScore           = 0.4
	DefaultEmbeddingBatchSize = 128
)

type ComponentConfig struct {
	Completion         string  `json:"completion"`
	Model              string  `json:"model,omitempty"`
	BatchSize          int     `json:"batch_size,omitempty"`
	EmbeddingBatchSize int     `json:"embedding_batch_size,omitempty"`
	Limit              int     `json:"limit,omitempty"`
	MaxMessages        int     `json:"max_messages,omitempty"`
	MaxOutputTokens    int     `json:"max_output_tokens,omitempty"`
	MinScore           float64 `json:"min_score,omitempty"`
	KeepWarmFor        string  `json:"keep_warm_for,omitempty"`
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

func saveComponentConfig(home string, config ComponentConfig) error {
	path := filepath.Join(strings.TrimSpace(home), ComponentConfigFilename)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create semantic component config dir: %w", err)
	}
	body, err := json.MarshalIndent(config.withDefaults(), "", "  ")
	if err != nil {
		return fmt.Errorf("encode semantic component config: %w", err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return fmt.Errorf("write semantic component config %s: %w", path, err)
	}
	return nil
}

func (c ComponentConfig) withDefaults() ComponentConfig {
	c.Completion = strings.TrimSpace(c.Completion)
	c.Model = strings.TrimSpace(c.Model)
	if c.BatchSize <= 0 {
		c.BatchSize = DefaultBatchSize
	}
	if c.EmbeddingBatchSize <= 0 {
		c.EmbeddingBatchSize = DefaultEmbeddingBatchSize
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
	c.KeepWarmFor = strings.TrimSpace(c.KeepWarmFor)
	return c
}
