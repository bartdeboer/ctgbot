package component

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

type Constructor func(
	ctx context.Context,
	registration coremodel.Component,
	runtime runtimepkg.Factory,
	profile runtimepkg.Profile,
	storage repository.Storage,
) (Component, error)

type Loaded struct {
	Registration coremodel.Component
	Profile      runtimepkg.Profile
	Runtime      runtimepkg.Factory
	Component    Component
}

type Registry struct {
	constructors map[string]Constructor
}

func NewRegistry() *Registry {
	return &Registry{constructors: map[string]Constructor{}}
}

func (r *Registry) Add(componentType string, constructor Constructor) error {
	if r == nil {
		return fmt.Errorf("missing component registry")
	}
	componentType = strings.TrimSpace(componentType)
	if componentType == "" {
		return fmt.Errorf("missing component type")
	}
	if constructor == nil {
		return fmt.Errorf("missing constructor for %s", componentType)
	}
	if r.constructors == nil {
		r.constructors = map[string]Constructor{}
	}
	if _, exists := r.constructors[componentType]; exists {
		return fmt.Errorf("component constructor already registered: %s", componentType)
	}
	r.constructors[componentType] = constructor
	return nil
}

func (r *Registry) Has(componentType string) bool {
	if r == nil {
		return false
	}
	_, ok := r.constructors[strings.TrimSpace(componentType)]
	return ok
}

func (r *Registry) Build(
	ctx context.Context,
	registration coremodel.Component,
	runtime runtimepkg.Factory,
	profile runtimepkg.Profile,
	storage repository.Storage,
) (*Loaded, error) {
	if r == nil {
		return nil, fmt.Errorf("missing component registry")
	}
	componentType := strings.TrimSpace(registration.Type)
	if componentType == "" {
		return nil, fmt.Errorf("missing component type")
	}
	constructor, ok := r.constructors[componentType]
	if !ok {
		return nil, fmt.Errorf("component constructor not registered: %s", componentType)
	}
	value, err := constructor(ctx, registration, runtime, profile, storage)
	if err != nil {
		return nil, err
	}
	return &Loaded{
		Registration: registration,
		Profile:      profile,
		Runtime:      runtime,
		Component:    value,
	}, nil
}
