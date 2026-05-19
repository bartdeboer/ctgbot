package claude

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
)

const claudeConfigModel = "model"

func (c *Component) ConfigSchema(ctx context.Context, req commandengine.Request) (configsurface.ConfigSchema, error) {
	_, _ = ctx, req
	return configsurface.ConfigSchema{Fields: []configsurface.FieldSchema{
		{
			Key:      claudeConfigModel,
			Help:     "Claude model for this thread",
			Type:     configsurface.FieldTypeString,
			Writable: true,
			Default:  c.defaultModel(),
		},
	}}, nil
}

func (c *Component) ConfigGet(ctx context.Context, req commandengine.Request, key string) (string, error) {
	thread, err := c.thread(ctx, req)
	if err != nil {
		return "", err
	}
	settings, err := c.resolveThreadSettings(ctx, thread)
	if err != nil {
		return "", err
	}
	switch configsurface.NormalizeKey(key) {
	case claudeConfigModel:
		return settings.Model, nil
	default:
		return "", unknownClaudeConfig(key)
	}
}

func (c *Component) ConfigSet(ctx context.Context, req commandengine.Request, key string, value string) error {
	thread, err := c.thread(ctx, req)
	if err != nil {
		return err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("missing config value")
	}
	switch configsurface.NormalizeKey(key) {
	case claudeConfigModel:
		return c.updateThreadState(ctx, thread, func(state *threadState) { state.Model = value })
	default:
		return unknownClaudeConfig(key)
	}
}

func (c *Component) ConfigUnset(ctx context.Context, req commandengine.Request, key string) error {
	thread, err := c.thread(ctx, req)
	if err != nil {
		return err
	}
	switch configsurface.NormalizeKey(key) {
	case claudeConfigModel:
		return c.updateThreadState(ctx, thread, func(state *threadState) { state.Model = "" })
	default:
		return unknownClaudeConfig(key)
	}
}

func (c *Component) defaultModel() string {
	if c == nil {
		return "(claude default)"
	}
	if model := strings.TrimSpace(c.componentConfig.Model); model != "" {
		return model
	}
	return "(claude default)"
}

func unknownClaudeConfig(key string) error {
	return fmt.Errorf("unknown claude config %q", configsurface.NormalizeKey(key))
}
