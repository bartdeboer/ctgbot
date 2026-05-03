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
	"github.com/bartdeboer/ctgbot/internal/v4/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v4/profiles"
	"github.com/bartdeboer/ctgbot/internal/v4/repository"
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
	Profile      profiles.Profile
	Home         Home
	Runtime      Runtime
	Storage      repository.Storage
}

type Instance struct {
	Registration   coremodel.Component
	Profile        profiles.Profile
	Home           Home
	Runtime        Runtime
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
	return r.InstantiateWithRuntime(ctx, registration, profiles.Profile{}, home, nil, storage)
}

func (r *Registry) InstantiateWithRuntime(ctx context.Context, registration coremodel.Component, profile profiles.Profile, home Home, runtime Runtime, storage repository.Storage) (*Instance, error) {
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
		Profile:      profile,
		Home:         home,
		Runtime:      runtime,
		Storage:      storage,
	})
	if err != nil {
		return nil, err
	}
	return &Instance{
		Registration:   registration,
		Profile:        profile,
		Home:           home,
		Runtime:        runtime,
		Implementation: impl,
	}, nil
}

type Runtime interface {
	Driver() string
	Profile() profiles.Profile
	ContainerWorkspace() string
	StartAuth(ctx context.Context, req RuntimeAuthRequest) (*sandboxengine.Sandbox, error)
	StartTurn(ctx context.Context, req RuntimeTurnRequest) (*sandboxengine.SandboxRuntime, error)
}

type RuntimeAuthRequest struct {
	Registration coremodel.Component
	Home         Home
	Image        string
	Workdir      string
	Env          []string
}

type RuntimeTurnRequest struct {
	Registration         coremodel.Component
	Home                 Home
	Thread               coremodel.Thread
	Image                string
	Workdir              string
	Env                  []string
	DeveloperInstructions string
	Commands             commandengine.CommandExecutor
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
	ComponentThreadID(componentID modeluuid.UUID) (string, bool, error)
	BindComponentThreadID(componentID modeluuid.UUID, componentThreadID string) error
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

	Stdout         io.Writer
	Stderr         io.Writer
}

type Authenticator interface {
	Component
	Auth(ctx context.Context, req AuthRequest) error
}
