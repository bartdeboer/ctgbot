package semantic

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
)

const (
	semanticConfigCompletion         = "completion"
	semanticConfigModel              = "model"
	semanticConfigBatchSize          = "batch-size"
	semanticConfigEmbeddingBatchSize = "embedding-batch-size"
	semanticConfigLimit              = "limit"
	semanticConfigMaxMessages        = "max-messages"
	semanticConfigMaxOutputTokens    = "max-output-tokens"
	semanticConfigMinScore           = "min-score"
	semanticConfigKeepWarmFor        = "keep-warm-for"
)

func (c *Component) ConfigSchema(ctx context.Context, req commandengine.Request) (configsurface.ConfigSchema, error) {
	_, _ = ctx, req
	defaults := ComponentConfig{}.withDefaults()
	return configsurface.ConfigSchema{Fields: []configsurface.FieldSchema{
		{Key: semanticConfigCompletion, Help: "Completion component ref for LLM scoring/explanations", Type: configsurface.FieldTypeString, Writable: true},
		{Key: semanticConfigModel, Help: "Default model name for semantic operations", Type: configsurface.FieldTypeString, Writable: true},
		{Key: semanticConfigBatchSize, Help: "LLM scoring batch size", Type: configsurface.FieldTypeInt, Writable: true, Default: strconv.Itoa(defaults.BatchSize)},
		{Key: semanticConfigEmbeddingBatchSize, Help: "Embedding index batch size", Type: configsurface.FieldTypeInt, Writable: true, Default: strconv.Itoa(defaults.EmbeddingBatchSize)},
		{Key: semanticConfigLimit, Help: "Default search result limit", Type: configsurface.FieldTypeInt, Writable: true, Default: strconv.Itoa(defaults.Limit)},
		{Key: semanticConfigMaxMessages, Help: "Default maximum messages to index/search", Type: configsurface.FieldTypeInt, Writable: true, Default: strconv.Itoa(defaults.MaxMessages)},
		{Key: semanticConfigMaxOutputTokens, Help: "Maximum LLM output tokens for semantic operations", Type: configsurface.FieldTypeInt, Writable: true, Default: strconv.Itoa(defaults.MaxOutputTokens)},
		{Key: semanticConfigMinScore, Help: "Minimum search score returned by default", Type: configsurface.FieldTypeFloat, Writable: true, Default: strconv.FormatFloat(defaults.MinScore, 'f', -1, 64)},
		{Key: semanticConfigKeepWarmFor, Help: "Keep inference sessions warm for this duration", Type: configsurface.FieldTypeDuration, Writable: true},
	}}, nil
}

func (c *Component) ConfigGet(ctx context.Context, req commandengine.Request, key string) (string, error) {
	_, _ = ctx, req
	config := c.currentConfig()
	switch configsurface.NormalizeKey(key) {
	case semanticConfigCompletion:
		return config.Completion, nil
	case semanticConfigModel:
		return config.Model, nil
	case semanticConfigBatchSize:
		return strconv.Itoa(config.BatchSize), nil
	case semanticConfigEmbeddingBatchSize:
		return strconv.Itoa(config.EmbeddingBatchSize), nil
	case semanticConfigLimit:
		return strconv.Itoa(config.Limit), nil
	case semanticConfigMaxMessages:
		return strconv.Itoa(config.MaxMessages), nil
	case semanticConfigMaxOutputTokens:
		return strconv.Itoa(config.MaxOutputTokens), nil
	case semanticConfigMinScore:
		return strconv.FormatFloat(config.MinScore, 'f', -1, 64), nil
	case semanticConfigKeepWarmFor:
		return config.KeepWarmFor, nil
	default:
		return "", unknownSemanticConfig(key)
	}
}

func (c *Component) ConfigSet(ctx context.Context, req commandengine.Request, key string, value string) error {
	_, _ = ctx, req
	config := c.currentConfig()
	key = configsurface.NormalizeKey(key)
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("missing config value")
	}
	switch key {
	case semanticConfigCompletion:
		config.Completion = value
	case semanticConfigModel:
		config.Model = value
	case semanticConfigBatchSize:
		parsed, err := positiveInt(key, value)
		if err != nil {
			return err
		}
		config.BatchSize = parsed
	case semanticConfigEmbeddingBatchSize:
		parsed, err := positiveInt(key, value)
		if err != nil {
			return err
		}
		config.EmbeddingBatchSize = parsed
	case semanticConfigLimit:
		parsed, err := positiveInt(key, value)
		if err != nil {
			return err
		}
		config.Limit = parsed
	case semanticConfigMaxMessages:
		parsed, err := positiveInt(key, value)
		if err != nil {
			return err
		}
		config.MaxMessages = parsed
	case semanticConfigMaxOutputTokens:
		parsed, err := positiveInt(key, value)
		if err != nil {
			return err
		}
		config.MaxOutputTokens = parsed
	case semanticConfigMinScore:
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("config %s expects positive float", key)
		}
		config.MinScore = parsed
	case semanticConfigKeepWarmFor:
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("config %s expects duration: %w", key, err)
		}
		config.KeepWarmFor = value
	default:
		return unknownSemanticConfig(key)
	}
	return c.saveConfig(config)
}

func (c *Component) ConfigUnset(ctx context.Context, req commandengine.Request, key string) error {
	_, _ = ctx, req
	config := c.currentConfig()
	switch configsurface.NormalizeKey(key) {
	case semanticConfigCompletion:
		config.Completion = ""
	case semanticConfigModel:
		config.Model = ""
	case semanticConfigBatchSize:
		config.BatchSize = 0
	case semanticConfigEmbeddingBatchSize:
		config.EmbeddingBatchSize = 0
	case semanticConfigLimit:
		config.Limit = 0
	case semanticConfigMaxMessages:
		config.MaxMessages = 0
	case semanticConfigMaxOutputTokens:
		config.MaxOutputTokens = 0
	case semanticConfigMinScore:
		config.MinScore = 0
	case semanticConfigKeepWarmFor:
		config.KeepWarmFor = ""
	default:
		return unknownSemanticConfig(key)
	}
	return c.saveConfig(config)
}

func (c *Component) currentConfig() ComponentConfig {
	if c == nil {
		return ComponentConfig{}.withDefaults()
	}
	return c.config.withDefaults()
}

func (c *Component) saveConfig(config ComponentConfig) error {
	if c == nil {
		return fmt.Errorf("missing semantic component")
	}
	config = config.withDefaults()
	if err := saveComponentConfig(c.homePath, config); err != nil {
		return err
	}
	c.config = config
	return nil
}

func positiveInt(key string, value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("config %s expects positive integer", key)
	}
	return parsed, nil
}

func unknownSemanticConfig(key string) error {
	return fmt.Errorf("unknown semantic config %q", configsurface.NormalizeKey(key))
}
