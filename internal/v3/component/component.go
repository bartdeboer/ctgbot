package component

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/ctgbot/internal/v3/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v3/repository"
)

type Home struct {
	HostPath      string
	ContainerPath string
}

type Component interface {
	Type() string
}

type Factory interface {
	Type() string
	Create(ctx context.Context, req CreateRequest) (Component, error)
}

type CreateRequest struct {
	Registration coremodel.Component
	Home         Home
	Storage      repository.Storage
}

type Instance struct {
	Registration   coremodel.Component
	Home           Home
	Implementation Component
}

type Registry struct {
	factories map[string]Factory
}

func NewRegistry(factories ...Factory) *Registry {
	r := &Registry{factories: map[string]Factory{}}
	for _, factory := range factories {
		_ = r.Add(factory)
	}
	return r
}

func (r *Registry) Add(factory Factory) error {
	if r == nil {
		return fmt.Errorf("missing registry")
	}
	if factory == nil {
		return fmt.Errorf("missing factory")
	}
	componentType := strings.TrimSpace(factory.Type())
	if componentType == "" {
		return fmt.Errorf("missing factory type")
	}
	if r.factories == nil {
		r.factories = map[string]Factory{}
	}
	if _, exists := r.factories[componentType]; exists {
		return fmt.Errorf("component factory already registered: %s", componentType)
	}
	r.factories[componentType] = factory
	return nil
}

func (r *Registry) Factory(componentType string) (Factory, bool) {
	if r == nil {
		return nil, false
	}
	factory, ok := r.factories[strings.TrimSpace(componentType)]
	return factory, ok
}

func (r *Registry) Instantiate(ctx context.Context, registration coremodel.Component, home Home, storage repository.Storage) (*Instance, error) {
	componentType := strings.TrimSpace(registration.Type)
	if componentType == "" {
		return nil, fmt.Errorf("missing component type")
	}
	factory, ok := r.Factory(componentType)
	if !ok {
		return nil, fmt.Errorf("component factory not registered: %s", componentType)
	}
	impl, err := factory.Create(ctx, CreateRequest{
		Registration: registration,
		Home:         home,
		Storage:      storage,
	})
	if err != nil {
		return nil, err
	}
	return &Instance{
		Registration:   registration,
		Home:           home,
		Implementation: impl,
	}, nil
}

type InboundEvent struct {
	ComponentID modeluuid.UUID
	ExternalID  string
	Payload     messenger.InboundPayload
}

type InboundEmitter func(ctx context.Context, event InboundEvent) error

type InboundSource interface {
	Component
	RunInbound(ctx context.Context, emit InboundEmitter) error
}

type OutboundRelay interface {
	Component
	Send(ctx context.Context, payload messenger.OutboundPayload) error
	StartChatAction(ctx context.Context, target messenger.ChatTarget, action messenger.ChatAction) (func(), error)
}

type Agent interface {
	Component
	HandleTurn(ctx context.Context, turn Turn) (*TurnResult, error)
}

type Turn struct {
	Chat    coremodel.Chat
	Thread  coremodel.Thread
	Inbound coremodel.ThreadMessage
	Runtime TurnRuntime
}

type TurnResult struct {
	Final *coremodel.ThreadMessage
}

type TurnRuntime interface {
	Commands() commandengine.CommandExecutor
	Send(ctx context.Context, payload messenger.OutboundPayload) error
	StartChatAction(ctx context.Context, action messenger.ChatAction) (func(), error)
	ComponentHome(componentID modeluuid.UUID) (Home, bool)
}

type CommandSurface interface {
	Component
	CommandDefinitions() []commandengine.Definition
	RegisterCommandHandlers(registry *commandengine.Registry) error
}

type ManagedFile struct {
	RelativePath string
	Required     bool
	Sensitive    bool
}

type ProfileOwner interface {
	Component
	ManagedFiles() []ManagedFile
}

type AuthRequest struct {
	Registration coremodel.Component
	Home         Home
	Image        string

	CallbackPort    int
	CallbackTimeout time.Duration

	SandboxManager sandboxengine.Manager
	Stdout         io.Writer
	Stderr         io.Writer
}

type Authenticator interface {
	Component
	Auth(ctx context.Context, req AuthRequest) error
}
