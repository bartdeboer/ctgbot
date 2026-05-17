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
	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
)

type Component interface {
	Type() string
}

// ChatPayloadSender is the narrow broker capability a command component needs
// when it wants to send a normal outbound chat payload, like hostbridge message,
// sendfile, or sendstdin do.
type ChatPayloadSender interface {
	SendPayload(ctx context.Context, threadID modeluuid.UUID, payload message.OutboundPayload) error
}

type ChatPayloadSenderReceiver interface {
	SetChatPayloadSender(sender ChatPayloadSender)
}

type SearchRequest struct {
	Query                 string
	Model                 string
	ChatID                modeluuid.UUID
	ThreadID              modeluuid.UUID
	Limit                 int
	BatchSize             int
	MaxMessages           int
	MinScore              float64
	CompletionIdleTimeout time.Duration
}

type SearchResponse struct {
	Results []SearchResult
}

type SearchResult struct {
	MessageID modeluuid.UUID
	ChatID    modeluuid.UUID
	ThreadID  modeluuid.UUID
	Excerpt   string
	Text      string
	Score     float64
	Reason    string
}

type Searcher interface {
	Component
	Search(ctx context.Context, req SearchRequest) (SearchResponse, error)
}

type SearchMessageSource interface {
	ForEachMessage(ctx context.Context, scope MessageScope, visit MessageVisitor) error
}

type SearchMessageSourceReceiver interface {
	SetSearchMessageSource(source SearchMessageSource)
}

type MessageScope struct {
	ChatID   modeluuid.UUID
	ThreadID modeluuid.UUID
	All      bool
	Limit    int
	Order    MessageOrder
	Kinds    []coremodel.MessageKind
}

type MessageVisitor func(coremodel.ThreadMessage) error

type MessageOrder string

const (
	MessageOrderOldestFirst MessageOrder = "oldest_first"
	MessageOrderNewestFirst MessageOrder = "newest_first"
)

type EmbeddingKind string

const (
	EmbeddingKindDocument EmbeddingKind = "document"
	EmbeddingKindQuery    EmbeddingKind = "query"
)

type EmbeddingInput struct {
	ID   string
	Text string
	Kind EmbeddingKind
}

type Embedding struct {
	ID         string
	Vector     []float32
	Dim        int
	Model      string
	Normalized bool
}

type EmbedRequest struct {
	Model  string
	Inputs []EmbeddingInput
}

type EmbedResponse struct {
	Embeddings []Embedding
}

type Embedder interface {
	Component
	Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error)
}

type TranscriptionRequest struct {
	Media message.Media
}

type TranscriptionResult struct {
	Text     string
	Language string
	Model    string
}

type Transcriber interface {
	Component
	Transcribe(ctx context.Context, req TranscriptionRequest) (TranscriptionResult, error)
}

type SpeechRequest struct {
	Text string
}

type SpeechResult struct {
	Media message.Media
}

type SpeechSynthesizer interface {
	Component
	Synthesize(ctx context.Context, req SpeechRequest) (SpeechResult, error)
}

type ModelMode string

const (
	ModelModeCompletion ModelMode = "completion"
	ModelModeEmbedding  ModelMode = "embedding"
)

type Model struct {
	Name        string
	URL         string
	Filename    string
	Path        string
	Mode        ModelMode
	SHA256      string
	MMProjPath  string
	HostPort    int
	ContextSize int
	UBatchSize  int
	GPULayers   int
	MaxTokens   int
	Temperature float64
	Pooling     string
	Normalize   bool
}

type ModelInstallRequest struct {
	Model
	Default bool
}

type ModelStore interface {
	Component
	ListModels(ctx context.Context) ([]Model, error)
	GetModel(ctx context.Context, name string) (Model, error)
	InstallModel(ctx context.Context, req ModelInstallRequest) (Model, error)
	RegisterModel(ctx context.Context, req ModelInstallRequest) (Model, error)
	DefaultModel(ctx context.Context) (string, error)
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

type ResolvedInboundQueuer interface {
	QueueResolvedInbound(ctx context.Context, inbound ResolvedInbound) error
}

type InboundSource interface {
	Component
	RunInbound(ctx context.Context, emit InboundEmitter) error
}

// SourceBindingDefaults lets source components provide their natural provider
// chat identifier when an operator binds them to a ctgbot chat. Components that
// cannot infer this value should not implement it.
type SourceBindingDefaults interface {
	Component
	DefaultSourceExternalChannelID(ctx context.Context) (string, error)
}

type OutboundRelay interface {
	Component
	Send(ctx context.Context, payload message.OutboundPayload) error
	StartChatAction(ctx context.Context, target message.ChatTarget, action message.ChatAction) (func(), error)
}

// MessageSendRequest is the component-direct message contract used by commands
// such as:
//
//	hostbridge gmail/personal message "hello" --to you@example.com
//	hostbridge telegram/telegram message "hello"
//
// It intentionally stays separate from message.OutboundPayload. OutboundPayload
// is the broker relay payload for an already-routed ctgbot thread; this request
// is for sending through a specific component and may need provider envelope
// fields such as recipients, subject, reply ids, or attachments.
//
// Keep common provider fields explicit instead of hiding them in a generic
// metadata map. Components should use the fields they understand and reject
// missing required fields for their provider.
type MessageSendRequest struct {
	To          []string
	Cc          []string
	Bcc         []string
	Subject     string
	Body        string
	ContentType string
	Syntax      string
	Attachments []message.Media
	ThreadID    string
	InReplyTo   string
}

type MessageSendResult struct {
	ID       string
	ThreadID string
}

type MessageSender interface {
	Component
	SendMessage(ctx context.Context, request MessageSendRequest) (MessageSendResult, error)
}

type Agent interface {
	Component
	HandleTurn(ctx context.Context, turn Turn) (*TurnResult, error)
}

// CompletionProvider receives a normalized completion prompt.
//
// The prompt shape intentionally aligns with OpenAI-style chat completions so
// that components such as llama.cpp can translate it almost directly to their
// backend payloads without needing to understand broker storage details.
type CompletionProvider interface {
	Component
	HandleCompletion(ctx context.Context, request CompletionRequest) (*CompletionResult, error)
}

// CompletionSession lets callers bracket several completion requests as one
// logical use of a provider. Providers with expensive warm state, such as a
// GPU-backed model server, can keep that state alive until the session closes.
type CompletionSession interface {
	Close() error
}

type CompletionSessionOptions struct {
	Model       string
	IdleTimeout time.Duration
}

type CompletionSessionProvider interface {
	Component
	BeginCompletionSession(ctx context.Context, options CompletionSessionOptions) (CompletionSession, error)
}

type CommandSurface interface {
	Component
	CommandDefinitions() []commandengine.Definition
	RegisterCommandHandlers(registry *commandengine.Registry) error
}

type Skill struct {
	Name        string
	Description string
	Text        string
}

type SkillProvider interface {
	Component
	Skill() Skill
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

type CompletionMode string

const (
	CompletionModeDefault    CompletionMode = ""
	CompletionModeRestricted CompletionMode = "restricted"
)

// CompletionRequest is intentionally reusable for both normal thread turns and
// bounded classifier-style completions. Restricted consumers should leave
// Runtime nil and use Mode/controls to make the intended execution envelope
// explicit to providers.
type CompletionRequest struct {
	Chat            coremodel.Chat
	Thread          coremodel.Thread
	Model           string
	Prompt          CompletionPrompt
	Runtime         TurnRuntime
	MaxOutputTokens int
	ResponseFormat  string
	Mode            CompletionMode
}

type CompletionResult struct {
	Final *coremodel.ThreadMessage
}

// RuntimeImageProvider lets components declare ctgbot-managed runtime/sandbox
// image targets. It is intentionally not a generic hook for arbitrary
// component-private Docker images; targets returned here are built by ctgbot's
// standard runtime image builder and may be refreshed by operator image
// commands such as `ctgbot image build`.
type RuntimeImageProvider interface {
	Component
	RuntimeImageTargets(ctx context.Context) ([]runtimeimage.Target, error)
}

type TurnInstructions struct {
	ChatProvider              string
	MessagePrefix             string
	KeepRepliesConcise        bool
	HostbridgeCommandNames    []string
	HostbridgeControlCommands []string
	RuntimeNotices            []string
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
