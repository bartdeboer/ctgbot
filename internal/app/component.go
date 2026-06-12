package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

type RegisterComponentResult struct {
	Component          coremodel.Component
	HostProfilePath    string
	RuntimeProfilePath string
}

type ComponentInfo struct {
	Component       coremodel.Component
	RuntimeKind     string
	HostProfilePath string
}

type UnregisterComponentResult struct {
	ComponentRef          string
	ComponentID           string
	ChatComponents        int64
	InboundFilterBindings int64
	ThreadMappings        int64
	ThreadStates          int64
	ComponentRemoved      bool
}

func (s *service) RegisterComponent(ctx context.Context, componentRef string, runtimeKind string, profilePath string) (RegisterComponentResult, error) {
	manager, err := s.componentManager()
	if err != nil {
		return RegisterComponentResult{}, err
	}
	registration, err := manager.EnsureComponent(ctx, strings.TrimSpace(componentRef), strings.TrimSpace(runtimeKind), strings.TrimSpace(profilePath))
	if err != nil {
		return RegisterComponentResult{}, err
	}
	if registration == nil {
		return RegisterComponentResult{}, fmt.Errorf("component registration returned nil")
	}
	return s.componentRegistrationResult(*registration)
}

func (s *service) UnregisterComponent(ctx context.Context, componentRef string) (UnregisterComponentResult, error) {
	if s == nil || s.Storage == nil {
		return UnregisterComponentResult{}, fmt.Errorf("missing app storage")
	}
	registration, err := s.resolveComponentRegistration(ctx, componentRef)
	if err != nil {
		return UnregisterComponentResult{}, err
	}
	if registration == nil {
		return UnregisterComponentResult{}, fmt.Errorf("component not found: %s", strings.TrimSpace(componentRef))
	}

	result := UnregisterComponentResult{
		ComponentRef: registration.Ref(),
		ComponentID:  registration.ID.String(),
	}
	componentID := registration.ID
	if err := s.Storage.Transaction(ctx, func(tx repository.Storage) error {
		sourceBindings, err := tx.ChatComponents().ListByComponentID(ctx, componentID)
		if err != nil {
			return err
		}
		sourceBindingIDs := make([]modeluuid.UUID, 0, len(sourceBindings))
		for _, binding := range sourceBindings {
			if !binding.ID.IsNull() {
				sourceBindingIDs = append(sourceBindingIDs, binding.ID)
			}
		}

		n, err := tx.InboundFilterBindings().DeleteBySourceBindingIDs(ctx, sourceBindingIDs)
		if err != nil {
			return err
		}
		result.InboundFilterBindings += n
		n, err = tx.InboundFilterBindings().DeleteByFilterComponentID(ctx, componentID)
		if err != nil {
			return err
		}
		result.InboundFilterBindings += n

		n, err = tx.ThreadComponentMappings().DeleteByComponentID(ctx, componentID)
		if err != nil {
			return err
		}
		result.ThreadMappings = n
		n, err = tx.ThreadComponentStates().DeleteByComponentID(ctx, componentID)
		if err != nil {
			return err
		}
		result.ThreadStates = n
		n, err = tx.ChatComponents().DeleteByComponentID(ctx, componentID)
		if err != nil {
			return err
		}
		result.ChatComponents = n
		removed, err := tx.Components().DeleteByID(ctx, componentID)
		if err != nil {
			return err
		}
		result.ComponentRemoved = removed
		return nil
	}); err != nil {
		return UnregisterComponentResult{}, err
	}
	if !result.ComponentRemoved {
		return UnregisterComponentResult{}, fmt.Errorf("component not found during unregister: %s", registration.Ref())
	}
	return result, nil
}

func (s *service) ListComponents(ctx context.Context) ([]ComponentInfo, error) {
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
		profile := runtime.ComponentProfile(registration)
		out = append(out, ComponentInfo{
			Component:       registration,
			RuntimeKind:     runtime.Kind(),
			HostProfilePath: profile.Path,
		})
	}
	return out, nil
}

func (s *service) componentRegistrationResult(registration coremodel.Component) (RegisterComponentResult, error) {
	manager, err := s.componentManager()
	if err != nil {
		return RegisterComponentResult{}, err
	}
	runtime, err := manager.Runtime(registration.Runtime)
	if err != nil {
		return RegisterComponentResult{}, err
	}
	profile := runtime.ComponentProfile(registration)
	return RegisterComponentResult{
		Component:          registration,
		HostProfilePath:    profile.Path,
		RuntimeProfilePath: runtime.RuntimeComponentProfilePath(registration, profile),
	}, nil
}
