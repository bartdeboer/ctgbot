package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

type ComponentGuardSetResult struct {
	Source  coremodel.Component
	Guard   coremodel.Component
	Binding coremodel.ComponentBinding
}

type ComponentGuardClearResult struct {
	Source   coremodel.Component
	Disabled int
}

type ComponentGuardStatusResult struct {
	Source   coremodel.Component
	Bindings []ComponentGuardStatusBinding
}

type ComponentGuardStatusBinding struct {
	Binding  coremodel.ComponentBinding
	GuardRef string
}

func (s *Service) SetComponentGuard(ctx context.Context, sourceRef string, guardRef string) (ComponentGuardSetResult, error) {
	if s == nil || s.Storage == nil {
		return ComponentGuardSetResult{}, fmt.Errorf("missing app storage")
	}
	source, err := s.resolveInboundSourceRegistration(ctx, sourceRef)
	if err != nil {
		return ComponentGuardSetResult{}, err
	}
	guard, err := s.resolveCompletionProviderRegistration(ctx, guardRef)
	if err != nil {
		return ComponentGuardSetResult{}, err
	}

	var binding coremodel.ComponentBinding
	if err := s.Storage.Transaction(ctx, func(tx repository.Storage) error {
		existing, err := tx.ComponentBindings().ListEnabledBySourceAndRole(ctx, source.ID, coremodel.ComponentBindingRoleGuard)
		if err != nil {
			return err
		}
		for _, old := range existing {
			old.Enabled = false
			if err := tx.ComponentBindings().Save(ctx, &old); err != nil {
				return err
			}
		}

		current, err := tx.ComponentBindings().GetBySourceTargetRole(ctx, source.ID, guard.ID, coremodel.ComponentBindingRoleGuard)
		if err != nil {
			return err
		}
		if current != nil {
			binding = *current
		} else {
			binding = coremodel.ComponentBinding{
				SourceComponentID: source.ID,
				TargetComponentID: guard.ID,
				Role:              coremodel.ComponentBindingRoleGuard,
			}
		}
		binding.Enabled = true
		return tx.ComponentBindings().Save(ctx, &binding)
	}); err != nil {
		return ComponentGuardSetResult{}, err
	}

	return ComponentGuardSetResult{Source: *source, Guard: *guard, Binding: binding}, nil
}

func (s *Service) ClearComponentGuard(ctx context.Context, sourceRef string) (ComponentGuardClearResult, error) {
	if s == nil || s.Storage == nil {
		return ComponentGuardClearResult{}, fmt.Errorf("missing app storage")
	}
	source, err := s.resolveInboundSourceRegistration(ctx, sourceRef)
	if err != nil {
		return ComponentGuardClearResult{}, err
	}

	disabled := 0
	if err := s.Storage.Transaction(ctx, func(tx repository.Storage) error {
		existing, err := tx.ComponentBindings().ListEnabledBySourceAndRole(ctx, source.ID, coremodel.ComponentBindingRoleGuard)
		if err != nil {
			return err
		}
		for _, binding := range existing {
			binding.Enabled = false
			if err := tx.ComponentBindings().Save(ctx, &binding); err != nil {
				return err
			}
			disabled++
		}
		return nil
	}); err != nil {
		return ComponentGuardClearResult{}, err
	}

	return ComponentGuardClearResult{Source: *source, Disabled: disabled}, nil
}

func (s *Service) ComponentGuardStatus(ctx context.Context, sourceRef string) (ComponentGuardStatusResult, error) {
	if s == nil || s.Storage == nil {
		return ComponentGuardStatusResult{}, fmt.Errorf("missing app storage")
	}
	source, err := s.resolveInboundSourceRegistration(ctx, sourceRef)
	if err != nil {
		return ComponentGuardStatusResult{}, err
	}
	bindings, err := s.Storage.ComponentBindings().ListEnabledBySourceAndRole(ctx, source.ID, coremodel.ComponentBindingRoleGuard)
	if err != nil {
		return ComponentGuardStatusResult{}, err
	}
	result := ComponentGuardStatusResult{
		Source:   *source,
		Bindings: make([]ComponentGuardStatusBinding, 0, len(bindings)),
	}
	for _, binding := range bindings {
		guardRef := binding.TargetComponentID.String()
		registration, err := s.Storage.Components().GetByID(ctx, binding.TargetComponentID)
		if err != nil {
			return ComponentGuardStatusResult{}, err
		}
		if registration != nil {
			guardRef = registration.Ref()
		}
		result.Bindings = append(result.Bindings, ComponentGuardStatusBinding{
			Binding:  binding,
			GuardRef: guardRef,
		})
	}
	return result, nil
}

func (s *Service) resolveInboundSourceRegistration(ctx context.Context, ref string) (*coremodel.Component, error) {
	if s == nil || s.Resolver == nil {
		return nil, fmt.Errorf("missing component resolver")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("missing source component ref")
	}
	registration, err := s.Resolver.ResolveComponentRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	loaded, err := s.Resolver.ResolveComponent(ctx, registration.ID)
	if err != nil {
		return nil, err
	}
	if _, ok := loaded.Component.(component.InboundSource); !ok {
		return nil, fmt.Errorf("component %s does not support inbound source", registration.Ref())
	}
	return registration, nil
}

func (s *Service) resolveCompletionProviderRegistration(ctx context.Context, ref string) (*coremodel.Component, error) {
	if s == nil || s.Resolver == nil {
		return nil, fmt.Errorf("missing component resolver")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("missing guard component ref")
	}
	registration, err := s.Resolver.ResolveComponentRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	loaded, err := s.Resolver.ResolveComponent(ctx, registration.ID)
	if err != nil {
		return nil, err
	}
	if _, ok := loaded.Component.(component.CompletionProvider); !ok {
		return nil, fmt.Errorf("component %s does not support completion provider guard", registration.Ref())
	}
	return registration, nil
}
