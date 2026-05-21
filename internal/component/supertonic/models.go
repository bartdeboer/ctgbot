package supertonic

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
)

func (c *Component) resolveModel(ctx context.Context, name string) (component.Model, error) {
	registry, err := c.modelRegistry(ctx)
	if err != nil {
		return component.Model{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name, err = registry.DefaultModelForMode(ctx, component.ModelModeTTS)
		if err != nil {
			return component.Model{}, err
		}
	}
	return registry.GetModel(ctx, strings.TrimSpace(name))
}

func (c *Component) modelRegistry(ctx context.Context) (component.ModelRegistry, error) {
	if c == nil {
		return nil, fmt.Errorf("missing supertonic component")
	}
	if c.resolver == nil {
		return nil, fmt.Errorf("missing component resolver")
	}
	ref := strings.TrimSpace(c.config.ModelRegistry)
	if ref == "" {
		ref = "model"
	}
	registration, err := c.resolver.ResolveComponentRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	loaded, err := c.resolver.ResolveComponent(ctx, registration.ID)
	if err != nil {
		return nil, err
	}
	if loaded == nil {
		return nil, fmt.Errorf("model registry not found: %s", ref)
	}
	registry, ok := loaded.Component.(component.ModelRegistry)
	if !ok {
		return nil, fmt.Errorf("component %s does not implement model registry", loaded.Registration.Ref())
	}
	return registry, nil
}
