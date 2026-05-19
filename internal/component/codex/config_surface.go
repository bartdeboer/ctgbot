package codex

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
)

const (
	codexConfigModel  = "model"
	codexConfigEffort = "effort"
)

func (c *Component) ConfigSchema(ctx context.Context, req commandengine.Request) (configsurface.ConfigSchema, error) {
	defaults := c.defaultThreadSettings()
	return configsurface.ConfigSchema{Fields: []configsurface.FieldSchema{
		{
			Key:      codexConfigModel,
			Help:     "Codex model for this thread",
			Type:     configsurface.FieldTypeString,
			Writable: true,
			Default:  defaults.Model,
			Options:  append([]string{}, suggestedCodexModels...),
		},
		{
			Key:      codexConfigEffort,
			Help:     "Codex reasoning effort for this thread",
			Type:     configsurface.FieldTypeEnum,
			Writable: true,
			Default:  defaults.ReasoningEffort,
			Options:  append([]string{}, suggestedCodexReasoningEfforts...),
		},
	}}, nil
}

func (c *Component) ConfigGet(ctx context.Context, req commandengine.Request, key string) (string, error) {
	thread, err := agentcommon.Thread(ctx, c.storage, req, Type)
	if err != nil {
		return "", err
	}
	settings, err := c.resolveThreadSettings(ctx, thread)
	if err != nil {
		return "", err
	}
	switch configsurface.NormalizeKey(key) {
	case codexConfigModel:
		return settings.Model, nil
	case codexConfigEffort:
		return settings.ReasoningEffort, nil
	default:
		return "", unknownCodexConfig(key)
	}
}

func (c *Component) ConfigSet(ctx context.Context, req commandengine.Request, key string, value string) error {
	thread, err := agentcommon.Thread(ctx, c.storage, req, Type)
	if err != nil {
		return err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("missing config value")
	}
	switch configsurface.NormalizeKey(key) {
	case codexConfigModel:
		return c.updateThreadState(ctx, thread, func(state *threadState) { state.Model = value })
	case codexConfigEffort:
		return c.updateThreadState(ctx, thread, func(state *threadState) { state.ReasoningEffort = value })
	default:
		return unknownCodexConfig(key)
	}
}

func (c *Component) ConfigUnset(ctx context.Context, req commandengine.Request, key string) error {
	thread, err := agentcommon.Thread(ctx, c.storage, req, Type)
	if err != nil {
		return err
	}
	switch configsurface.NormalizeKey(key) {
	case codexConfigModel:
		return c.updateThreadState(ctx, thread, func(state *threadState) { state.Model = "" })
	case codexConfigEffort:
		return c.updateThreadState(ctx, thread, func(state *threadState) { state.ReasoningEffort = "" })
	default:
		return unknownCodexConfig(key)
	}
}

func (c *Component) defaultThreadSettings() resolvedThreadSettings {
	settings := resolvedThreadSettings{
		Model:           "(codex default)",
		ReasoningEffort: "(codex default)",
	}
	if c == nil {
		return settings
	}
	if model := strings.TrimSpace(c.componentConfig.Model); model != "" {
		settings.Model = model
	} else if c.config != nil {
		if model := strings.TrimSpace(c.config.Codex().Model()); model != "" {
			settings.Model = model
		}
	}
	if effort := strings.TrimSpace(c.componentConfig.ReasoningEffort); effort != "" {
		settings.ReasoningEffort = effort
	}
	return settings
}

func unknownCodexConfig(key string) error {
	return fmt.Errorf("unknown codex config %q", configsurface.NormalizeKey(key))
}
