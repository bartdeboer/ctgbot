package chatbroker

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/agent"
	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

type PromptOutcome struct {
	Thread  *Thread
	Started bool
	Reply   string
}

const helpText = "Commands:\n/new [absolute-host-path]\n/refresh\n/purge\n/status\n/stop\n/upgrade\n/quit\n/help\n\nAny non-command message is sent to the active Codex conversation."

type Broker struct {
	Config            *appstate.Config
	Sessions          SessionStore
	Sandboxes         sandboxengine.Manager
	Dispatch          *Dispatcher
	ProcessActions    ProcessActions
	Agents            map[string]agent.Agent
	InboundProviders  map[string]messenger.InboundChatProvider
	OutboundProviders map[string]messenger.OutboundChatProvider
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
		Agents:            map[string]agent.Agent{},
		InboundProviders:  map[string]messenger.InboundChatProvider{},
		OutboundProviders: map[string]messenger.OutboundChatProvider{},
		DefaultAgent:      "codex",
		Logger:            logger,
	}
}

func (b *Broker) RegisterAgent(name string, agentImpl agent.Agent) {
	if b.Agents == nil {
		b.Agents = map[string]agent.Agent{}
	}
	b.Agents[name] = agentImpl
}

func (b *Broker) RegisterInboundChatProvider(name string, provider messenger.InboundChatProvider) {
	if b.InboundProviders == nil {
		b.InboundProviders = map[string]messenger.InboundChatProvider{}
	}
	b.InboundProviders[name] = provider
}

func (b *Broker) RegisterOutboundChatProvider(name string, provider messenger.OutboundChatProvider) {
	if b.OutboundProviders == nil {
		b.OutboundProviders = map[string]messenger.OutboundChatProvider{}
	}
	b.OutboundProviders[name] = provider
}

func (b *Broker) AutoMigrate(ctx context.Context) error {
	if b.Sessions == nil {
		return nil
	}
	return b.Sessions.AutoMigrate(ctx)
}

func (b *Broker) agent(name string) (agent.Agent, error) {
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

func (b *Broker) startThreadChatAction(ctx context.Context, thread *Thread, action messenger.ChatAction) func() {
	stop := func() {}
	if b == nil || b.Config == nil || thread == nil || thread.ChatID.IsNull() {
		return stop
	}
	chatCfg, err := b.Config.FindChatByID(thread.ChatID)
	if err != nil || chatCfg == nil {
		return stop
	}
	provider, ok := b.OutboundProviders[strings.TrimSpace(chatCfg.ProviderType)]
	if !ok || provider == nil {
		return stop
	}
	stop, err = provider.StartChatAction(ctx, messenger.ChatTarget{
		ProviderChatID:   strings.TrimSpace(chatCfg.ProviderChatID),
		ProviderThreadID: strings.TrimSpace(thread.ProviderThreadID),
	}, action)
	if err != nil {
		b.logf("start chat action failed chat=%s thread=%s action=%q err=%v", chatCfg.ID, thread.ID, action, err)
		return func() {}
	}
	if stop == nil {
		return func() {}
	}
	return stop
}
