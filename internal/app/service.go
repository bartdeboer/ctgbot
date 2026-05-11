package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

type ComponentResolver interface {
	ResolveComponentRef(ctx context.Context, ref string) (*coremodel.Component, error)
	ResolveComponent(ctx context.Context, id modeluuid.UUID) (*component.Loaded, error)
}

type WorkspaceValidator interface {
	ValidateWorkspace(name string) error
}

type ComponentManager interface {
	EnsureComponent(ctx context.Context, ref string, runtimeKind string, homePath string) (*coremodel.Component, error)
	Runtime(kind string) (runtimepkg.Factory, error)
}

type Service struct {
	Storage            repository.Storage
	Resolver           ComponentResolver
	ComponentManager   ComponentManager
	WorkspaceValidator WorkspaceValidator
}

func NewService(storage repository.Storage, resolver ComponentResolver) *Service {
	service := &Service{Storage: storage, Resolver: resolver}
	if manager, ok := resolver.(ComponentManager); ok {
		service.ComponentManager = manager
	}
	if validator, ok := resolver.(WorkspaceValidator); ok {
		service.WorkspaceValidator = validator
	}
	return service
}

func (s *Service) componentManager() (ComponentManager, error) {
	if s == nil || s.ComponentManager == nil {
		return nil, fmt.Errorf("missing component manager")
	}
	return s.ComponentManager, nil
}

func (s *Service) resolveComponentRegistration(ctx context.Context, ref string) (*coremodel.Component, error) {
	if s == nil || s.Resolver == nil {
		return nil, fmt.Errorf("missing component resolver")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("missing component ref")
	}
	return s.Resolver.ResolveComponentRef(ctx, ref)
}

func (s *Service) resolveLoadedComponent(ctx context.Context, id modeluuid.UUID) (*component.Loaded, error) {
	if s == nil || s.Resolver == nil {
		return nil, fmt.Errorf("missing component resolver")
	}
	return s.Resolver.ResolveComponent(ctx, id)
}
