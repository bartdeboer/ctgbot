package claude

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
)

const (
	claudeConfigModel                = "model"
	claudeConfigContainerKeepRunning = "container.keep-running"
)

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
		{
			Key:      claudeConfigContainerKeepRunning,
			Help:     "Keep the Claude runtime container running between turns",
			Type:     configsurface.FieldTypeBool,
			Writable: true,
			Default:  "false",
			Options:  []string{"true", "false"},
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
	case claudeConfigContainerKeepRunning:
		return strconv.FormatBool(settings.KeepRunning), nil
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
	case claudeConfigContainerKeepRunning:
		parsed, err := configsurface.ParseBool(value)
		if err != nil {
			return fmt.Errorf("config %s expects true or false", claudeConfigContainerKeepRunning)
		}
		return c.updateThreadState(ctx, thread, func(state *threadState) { state.KeepRunning = &parsed })
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
	case claudeConfigContainerKeepRunning:
		return c.updateThreadState(ctx, thread, func(state *threadState) { state.KeepRunning = nil })
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
