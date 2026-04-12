package chatbroker

import (
	"context"
	"fmt"
	"log"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

type TurnResult struct {
	Reply            string
	ProviderThreadID string
}

type Agent interface {
	Name() string
	SetupEnvironment(ctx context.Context, sbx *sandboxengine.Sandbox) error
	HandleTurn(ctx context.Context, sbx *sandboxengine.Sandbox, providerThreadID string, prompt string) (TurnResult, error)
}

type PurgingAgent interface {
	Purge(ctx context.Context, sbx *sandboxengine.Sandbox, providerThreadID string) error
}

type SkillInstallingAgent interface {
	InstallSkill(ctx context.Context, sbx *sandboxengine.Sandbox, skillDir string) error
}

type PromptOutcome struct {
	Thread  *Thread
	Started bool
	Reply   string
}

type IncomingMessage struct {
	ProviderType      string
	ProviderChatID    string
	ProviderThreadID  string
	Message           string
	ChatLabel         string
	UserLabel         string
	ProviderMessageID string
}

type IncomingAttachment struct {
	Kind     string
	Filename string
	Content  []byte
}

type IncomingUpdate struct {
	ProviderType      string
	ProviderChatID    string
	ProviderThreadID  string
	ProviderMessageID string
	ChatLabel         string
	UserLabel         string
	Text              string
	Attachments       []IncomingAttachment
}

type OutboundMessage struct {
	Text string
}

type OutgoingFile struct {
	SandboxID modeluuid.UUID
	Filename  string
	Caption   string
	Content   []byte
}

type ResolvedOutgoingMessage struct {
	ProviderChatID   string
	ProviderThreadID string
	Text             string
}

type ResolvedOutgoingFile struct {
	ProviderChatID   string
	ProviderThreadID string
	Filename         string
	Caption          string
	Content          []byte
}

type IncomingResult struct {
	Messages []OutboundMessage
}

type InboundChatProvider interface {
	ProviderType() string
	Run(ctx context.Context, onUpdate func(context.Context, IncomingUpdate) (IncomingResult, error)) error
}

type OutboundChatProvider interface {
	ProviderType() string
	SendText(ctx context.Context, msg ResolvedOutgoingMessage) error
	SendFile(ctx context.Context, file ResolvedOutgoingFile) error
}

const helpText = "Commands:\n/new [absolute-host-path]\n/refresh\n/purge\n/status\n/stop\n/upgrade\n/quit\n/help\n\nAny non-command message is sent to the active Codex conversation."

type Broker struct {
	Config            *appstate.Config
	Sessions          SessionStore
	Sandboxes         sandboxengine.Manager
	Dispatch          *Dispatcher
	ProcessActions    ProcessActions
	Agents            map[string]Agent
	InboundProviders  map[string]InboundChatProvider
	OutboundProviders map[string]OutboundChatProvider
	DefaultAgent      string
	Logger            *log.Logger
}

func New(cfg *appstate.Config, sessions SessionStore, sandboxes sandboxengine.Manager, logger *log.Logger) *Broker {
	if sandboxes == nil {
		sandboxes = sandboxengine.NewSandboxManager(logger)
	}
	return &Broker{
		Config:            cfg,
		Sessions:          sessions,
		Sandboxes:         sandboxes,
		Dispatch:          NewDispatcher(),
		Agents:            map[string]Agent{},
		InboundProviders:  map[string]InboundChatProvider{},
		OutboundProviders: map[string]OutboundChatProvider{},
		DefaultAgent:      "codex",
		Logger:            logger,
	}
}

func (b *Broker) RegisterAgent(name string, agent Agent) {
	if b.Agents == nil {
		b.Agents = map[string]Agent{}
	}
	b.Agents[name] = agent
}

func (b *Broker) RegisterInboundChatProvider(name string, provider InboundChatProvider) {
	if b.InboundProviders == nil {
		b.InboundProviders = map[string]InboundChatProvider{}
	}
	b.InboundProviders[name] = provider
}

func (b *Broker) RegisterOutboundChatProvider(name string, provider OutboundChatProvider) {
	if b.OutboundProviders == nil {
		b.OutboundProviders = map[string]OutboundChatProvider{}
	}
	b.OutboundProviders[name] = provider
}

func (b *Broker) AutoMigrate(ctx context.Context) error {
	if b.Sessions == nil {
		return nil
	}
	return b.Sessions.AutoMigrate(ctx)
}

func (b *Broker) agent(name string) (Agent, error) {
	if name == "" {
		name = b.defaultAgentName()
	}
	agent, ok := b.Agents[name]
	if !ok {
		return nil, fmt.Errorf("unknown agent provider %q", name)
	}
	return agent, nil
}

func (b *Broker) defaultAgentName() string {
	if b.DefaultAgent != "" {
		return b.DefaultAgent
	}
	return "codex"
}

func (b *Broker) sandboxManager() sandboxengine.Manager {
	if b.Sandboxes == nil {
		b.Sandboxes = sandboxengine.NewSandboxManager(b.Logger)
	}
	return b.Sandboxes
}

func (b *Broker) dispatcher() *Dispatcher {
	if b.Dispatch == nil {
		b.Dispatch = NewDispatcher()
	}
	return b.Dispatch
}

func (b *Broker) dispatchKey(chatID modeluuid.UUID, threadID modeluuid.UUID) dispatchKey {
	return dispatchKey{
		ChatID:   chatID,
		ThreadID: threadID,
	}
}

func threadIDOrNil(thread *Thread) modeluuid.UUID {
	if thread == nil {
		return modeluuid.Nil
	}
	return thread.ID
}

func (b *Broker) logf(format string, args ...any) {
	if b.Logger != nil {
		b.Logger.Printf(format, args...)
	}
}
