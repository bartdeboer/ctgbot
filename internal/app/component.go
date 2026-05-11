package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

type RegisterComponentResult struct {
	Component       coremodel.Component
	HostHomePath    string
	RuntimeHomePath string
}

type ComponentInfo struct {
	Component    coremodel.Component
	RuntimeKind  string
	HostHomePath string
}

func (s *Service) RegisterComponent(ctx context.Context, componentRef string, runtimeKind string, homePath string) (RegisterComponentResult, error) {
	manager, err := s.componentManager()
	if err != nil {
		return RegisterComponentResult{}, err
	}
	registration, err := manager.EnsureComponent(ctx, strings.TrimSpace(componentRef), strings.TrimSpace(runtimeKind), strings.TrimSpace(homePath))
	if err != nil {
		return RegisterComponentResult{}, err
	}
	if registration == nil {
		return RegisterComponentResult{}, fmt.Errorf("component registration returned nil")
	}
	return s.componentRegistrationResult(*registration)
}

func (s *Service) ListComponents(ctx context.Context) ([]ComponentInfo, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing app storage")
	}
	manager, err := s.componentManager()
	if err != nil {
		return nil, err
	}
	components, err := s.Storage.Components().ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ComponentInfo, 0, len(components))
	for _, registration := range components {
		runtime, err := manager.Runtime(registration.Runtime)
		if err != nil {
			return nil, err
		}
		home := runtime.ComponentHome(registration)
		out = append(out, ComponentInfo{
			Component:    registration,
			RuntimeKind:  runtime.Kind(),
			HostHomePath: home.Path,
		})
	}
	return out, nil
}

func (s *Service) componentRegistrationResult(registration coremodel.Component) (RegisterComponentResult, error) {
	manager, err := s.componentManager()
	if err != nil {
		return RegisterComponentResult{}, err
	}
	runtime, err := manager.Runtime(registration.Runtime)
	if err != nil {
		return RegisterComponentResult{}, err
	}
	home := runtime.ComponentHome(registration)
	return RegisterComponentResult{
		Component:       registration,
		HostHomePath:    home.Path,
		RuntimeHomePath: runtime.RuntimeComponentHomePath(registration, home),
	}, nil
}
