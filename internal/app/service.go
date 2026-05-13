package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
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

type ChatRuntimeResolver interface {
	ResolveChatWorkspace(ctx context.Context, chat coremodel.Chat) (string, error)
	ResolveChatHostbridgeAllowedCommands(ctx context.Context, chat coremodel.Chat) (map[string]hostbridgeserver.AllowedCommand, error)
}

type Service struct {
	Storage             repository.Storage
	Resolver            ComponentResolver
	ComponentManager    ComponentManager
	ChatRuntimeResolver ChatRuntimeResolver
	WorkspaceValidator  WorkspaceValidator
	Logf                func(format string, args ...any)
}

func NewService(storage repository.Storage, resolver ComponentResolver) *Service {
	return NewServiceWithLogger(storage, resolver, nil)
}

func NewServiceWithLogger(storage repository.Storage, resolver ComponentResolver, logf func(format string, args ...any)) *Service {
	service := &Service{Storage: storage, Resolver: resolver, Logf: logf}
	if manager, ok := resolver.(ComponentManager); ok {
		service.ComponentManager = manager
	}
	if runtimeResolver, ok := resolver.(ChatRuntimeResolver); ok {
		service.ChatRuntimeResolver = runtimeResolver
	}
	if validator, ok := resolver.(WorkspaceValidator); ok {
		service.WorkspaceValidator = validator
	}
	return service
}

func (s *Service) Repository() repository.Storage {
	if s == nil {
		return nil
	}
	return s.Storage
}

func (s *Service) ResolveComponent(ctx context.Context, componentID modeluuid.UUID) (*component.Loaded, error) {
	return s.resolveLoadedComponent(ctx, componentID)
}

func (s *Service) ResolveChatWorkspace(ctx context.Context, chat coremodel.Chat) (string, error) {
	if s == nil || s.ChatRuntimeResolver == nil {
		return "", fmt.Errorf("missing chat runtime resolver")
	}
	return s.ChatRuntimeResolver.ResolveChatWorkspace(ctx, chat)
}

func (s *Service) ResolveChatHostbridgeAllowedCommands(ctx context.Context, chat coremodel.Chat) (map[string]hostbridgeserver.AllowedCommand, error) {
	if s == nil || s.ChatRuntimeResolver == nil {
		return nil, fmt.Errorf("missing chat runtime resolver")
	}
	return s.ChatRuntimeResolver.ResolveChatHostbridgeAllowedCommands(ctx, chat)
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
