package guard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	Type                    = "guard"
	ComponentConfigFilename = "component.json"

	defaultMaxOutputTokens = 512
	defaultHighRiskScore   = 0.70
	maxInputRunes          = 12000
	maxAttachments         = 20
	filterPrecedence       = 2000
)

type ComponentConfig struct {
	Completion      string  `json:"completion"`
	MaxOutputTokens int     `json:"max_output_tokens,omitempty"`
	HighRiskScore   float64 `json:"high_risk_score,omitempty"`
}

func loadComponentConfig(homePath string) (ComponentConfig, error) {
	path := filepath.Join(strings.TrimSpace(homePath), ComponentConfigFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ComponentConfig{}.withDefaults(), nil
		}
		return ComponentConfig{}, fmt.Errorf("read guard component config %s: %w", path, err)
	}
	var config ComponentConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return ComponentConfig{}, fmt.Errorf("read guard component config %s: %w", path, err)
	}
	return config.withDefaults(), nil
}

func (c ComponentConfig) withDefaults() ComponentConfig {
	c.Completion = strings.TrimSpace(c.Completion)
	if c.MaxOutputTokens <= 0 {
		c.MaxOutputTokens = defaultMaxOutputTokens
	}
	if c.HighRiskScore <= 0 || c.HighRiskScore > 1 {
		c.HighRiskScore = defaultHighRiskScore
	}
	return c
}
