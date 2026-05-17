package whispercpp

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
)

func (c *Component) resolveModel(ctx context.Context, name string) (component.Model, error) {
	store, err := c.modelStore(ctx)
	if err != nil {
		return component.Model{}, err
	}
	return store.GetModel(ctx, strings.TrimSpace(name))
}

func (c *Component) modelStore(ctx context.Context) (component.ModelStore, error) {
	if c == nil {
		return nil, fmt.Errorf("missing whispercpp component")
	}
	if c.resolver == nil {
		return nil, fmt.Errorf("missing component resolver")
	}
	ref := strings.TrimSpace(c.config.ModelStore)
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
		return nil, fmt.Errorf("model store not found: %s", ref)
	}
	store, ok := loaded.Component.(component.ModelStore)
	if !ok {
		return nil, fmt.Errorf("component %s does not implement model store", loaded.Registration.Ref())
	}
	return store, nil
}
