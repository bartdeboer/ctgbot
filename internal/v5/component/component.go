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
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
)

type Component interface {
	Type() string
}

type Constructor func(
	ctx context.Context,
	registration coremodel.Component,
	runtime v5runtime.Factory,
	home v5runtime.Home,
	storage repository.Storage,
) (Component, error)

type Loaded struct {
	Registration coremodel.Component
	Home         v5runtime.Home
	Runtime      v5runtime.Factory
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
	runtime v5runtime.Factory,
	home v5runtime.Home,
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
	value, err := constructor(ctx, registration, runtime, home, storage)
	if err != nil {
		return nil, err
	}
	return &Loaded{
		Registration: registration,
		Home:         home,
		Runtime:      runtime,
		Component:    value,
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

// CompletionAgent receives the broker's conversation-shaped view of a thread.
//
// Implementations are responsible for translating that stable broker model to
// their backend-specific request format. For example, a llama.cpp component can
// turn the messages into OpenAI-compatible chat JSON while a future backend can
// choose a different wire format without changing the broker.
type CompletionAgent interface {
	Component
	HandleCompletion(ctx context.Context, request CompletionRequest) (*CompletionResult, error)
}

type CommandSurface interface {
	Component
	CommandDefinitions() []commandengine.Definition
	RegisterCommandHandlers(registry *commandengine.Registry) error
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

type CompletionRequest struct {
	Chat     coremodel.Chat
	Thread   coremodel.Thread
	Inbound  coremodel.ThreadMessage
	Messages []coremodel.ThreadMessage
	Runtime  TurnRuntime
}

type CompletionResult struct {
	Final *coremodel.ThreadMessage
}

type TurnInstructions struct {
	ChatProvider              string
	MessagePrefix             string
	KeepRepliesConcise        bool
	HostbridgeCommandNames    []string
	HostbridgeControlCommands []string
}

type TurnRuntime interface {
	Commands() commandengine.CommandExecutor
	Instructions() TurnInstructions
	Send(ctx context.Context, payload messenger.OutboundPayload) error
	StartChatAction(ctx context.Context, action messenger.ChatAction) (func(), error)
	WorkspacePath() string
	ComponentHome(componentID modeluuid.UUID) (v5runtime.Home, bool)
	ComponentThreadID(componentID modeluuid.UUID) (string, bool, error)
	BindComponentThreadID(componentID modeluuid.UUID, componentThreadID string) error
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

type Authenticator interface {
	Component
	Auth(ctx context.Context, callbackPort int, callbackTimeout time.Duration, stdout io.Writer, stderr io.Writer) error
}
