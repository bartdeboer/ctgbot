package chatbroker

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/bartdeboer/ctgbot/internal/agent"
	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/configcommands"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

type PromptOutcome struct {
	Thread  *Thread
	Started bool
	Reply   string
}

type Broker struct {
	Config            *appstate.Config
	Sessions          SessionStore
	Sandboxes         sandboxengine.Manager
	Dispatch          *Dispatcher
	ProcessActions    ProcessActions
	ConfigCommands    *configcommands.Service
	Agents            map[string]agent.Agent
	InboundProviders  map[string]messenger.InboundChatProvider
	OutboundProviders map[string]messenger.OutboundChatProvider
	DefaultAgent      string
	Logger            *log.Logger

	activeRunsMu sync.Mutex
	activeRuns   map[modeluuid.UUID]context.CancelFunc
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

func (b *Broker) sendThreadText(ctx context.Context, thread *Thread, text string) error {
	if b == nil || b.Config == nil || thread == nil || thread.ChatID.IsNull() || strings.TrimSpace(text) == "" {
		return nil
	}
	chatCfg, err := b.Config.FindChatByID(thread.ChatID)
	if err != nil || chatCfg == nil {
		return err
	}
	provider, ok := b.OutboundProviders[strings.TrimSpace(chatCfg.ProviderType)]
	if !ok || provider == nil {
		return fmt.Errorf("outbound provider not registered: %s", chatCfg.ProviderType)
	}
	return provider.Send(ctx, messenger.OutboundPayload{
		ProviderChatID:   strings.TrimSpace(chatCfg.ProviderChatID),
		ProviderThreadID: strings.TrimSpace(thread.ProviderThreadID),
		Text:             messenger.TextMessage{Text: text},
	})
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

func (b *Broker) setActiveRun(threadID modeluuid.UUID, cancel context.CancelFunc) {
	if b == nil || threadID.IsNull() || cancel == nil {
		return
	}
	b.activeRunsMu.Lock()
	defer b.activeRunsMu.Unlock()
	if b.activeRuns == nil {
		b.activeRuns = map[modeluuid.UUID]context.CancelFunc{}
	}
	b.activeRuns[threadID] = cancel
}

func (b *Broker) clearActiveRun(threadID modeluuid.UUID, cancel context.CancelFunc) {
	if b == nil || threadID.IsNull() {
		return
	}
	b.activeRunsMu.Lock()
	defer b.activeRunsMu.Unlock()
	if b.activeRuns == nil {
		return
	}
	current := b.activeRuns[threadID]
	if cancel != nil && current != nil {
		if fmt.Sprintf("%p", current) != fmt.Sprintf("%p", cancel) {
			return
		}
	}
	delete(b.activeRuns, threadID)
}

func (b *Broker) interruptThread(threadID modeluuid.UUID, sbx *sandboxengine.Sandbox) bool {
	if b == nil || threadID.IsNull() {
		return false
	}
	b.activeRunsMu.Lock()
	cancel := b.activeRuns[threadID]
	b.activeRunsMu.Unlock()
	if cancel == nil {
		return false
	}
	if sbx != nil {
		_ = sbx.Interrupt()
	}
	cancel()
	return true
}
