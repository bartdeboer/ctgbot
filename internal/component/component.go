package component

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

type Component interface {
	Type() string
}

type Constructor func(
	ctx context.Context,
	registration coremodel.Component,
	runtime runtimepkg.Factory,
	home runtimepkg.Home,
	storage repository.Storage,
) (Component, error)

type Loaded struct {
	Registration coremodel.Component
	Home         runtimepkg.Home
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
	home runtimepkg.Home,
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
	Payload     message.InboundPayload
}

type InboundEmitter func(ctx context.Context, event InboundEvent) error

type InboundPromptContext struct {
	Kind      string
	FromLabel string
	FromID    string
	ReplyHint string
}

type ResolvedInbound struct {
	Chat          coremodel.Chat
	Thread        coremodel.Thread
	ComponentID   modeluuid.UUID
	ExternalID    string
	Payload       message.InboundPayload
	Metadata      []string
	PromptContext *InboundPromptContext
}

type DeliveryResult struct {
	Inbound  *coremodel.ThreadMessage
	Outbound []coremodel.ThreadMessage
}

type ResolvedInboundHandler interface {
	HandleResolvedInbound(ctx context.Context, inbound ResolvedInbound) (DeliveryResult, error)
}

type InboundSource interface {
	Component
	RunInbound(ctx context.Context, emit InboundEmitter) error
}

type OutboundRelay interface {
	Component
	Send(ctx context.Context, payload message.OutboundPayload) error
	StartChatAction(ctx context.Context, target message.ChatTarget, action message.ChatAction) (func(), error)
}

type Agent interface {
	Component
	HandleTurn(ctx context.Context, turn Turn) (*TurnResult, error)
}

// CompletionAgent receives a normalized completion prompt for a thread.
//
// The prompt shape intentionally aligns with OpenAI-style chat completions so
// that components such as llama.cpp can translate it almost directly to their
// backend payloads without needing to understand broker storage details.
type CompletionAgent interface {
	Component
	HandleCompletion(ctx context.Context, request CompletionRequest) (*CompletionResult, error)
}

type CommandSurface interface {
	Component
	CommandDefinitions() []commandengine.Definition
	RegisterCommandHandlers(registry *commandengine.Registry) error
}

type LocalCommandSurface interface {
	CommandSurface
	UsesLocalCommandRoutes() bool
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

type CompletionRole string

const (
	CompletionRoleSystem    CompletionRole = "system"
	CompletionRoleDeveloper CompletionRole = "developer"
	CompletionRoleUser      CompletionRole = "user"
	CompletionRoleAssistant CompletionRole = "assistant"
	CompletionRoleTool      CompletionRole = "tool"
)

type CompletionMessage struct {
	Role    CompletionRole
	Content string
}

type CompletionPrompt struct {
	Messages []CompletionMessage
}

type CompletionRequest struct {
	Chat    coremodel.Chat
	Thread  coremodel.Thread
	Prompt  CompletionPrompt
	Runtime TurnRuntime
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
	Send(ctx context.Context, payload message.OutboundPayload) error
	StartChatAction(ctx context.Context, action message.ChatAction) (func(), error)
	WorkspacePath() string
	ComponentHome(componentID modeluuid.UUID) (runtimepkg.Home, bool)
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

type AuthStatusReporter interface {
	Component
	AuthStatus(ctx context.Context, stdout io.Writer, stderr io.Writer) error
}
