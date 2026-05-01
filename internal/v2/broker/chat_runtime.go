package broker

import (
	"context"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v2/component"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

type ChatRuntime struct {
	ChatID modeluuid.UUID

	Components []component.Component

	MessageCommands *commandengine.Engine
	AgentCommands   *commandengine.Engine

	Agents []component.Agent
	Relays []component.OutboundRelay

	Fingerprint string
}

func (b *Broker) runtimeForChat(ctx context.Context, chatID modeluuid.UUID) (*ChatRuntime, error) {
	bindings, err := b.enabledChatComponents(ctx, chatID)
	if err != nil {
		return nil, err
	}
	fingerprint := chatRuntimeFingerprint(bindings)

	b.runtimeMu.Lock()
	if b.runtimes == nil {
		b.runtimes = map[modeluuid.UUID]*ChatRuntime{}
	}
	if cached := b.runtimes[chatID]; cached != nil && cached.Fingerprint == fingerprint {
		b.runtimeMu.Unlock()
		return cached, nil
	}
	b.runtimeMu.Unlock()

	runtime, err := b.buildChatRuntime(chatID, bindings, fingerprint)
	if err != nil {
		return nil, err
	}

	b.runtimeMu.Lock()
	if b.runtimes == nil {
		b.runtimes = map[modeluuid.UUID]*ChatRuntime{}
	}
	b.runtimes[chatID] = runtime
	b.runtimeMu.Unlock()
	return runtime, nil
}

func (b *Broker) buildChatRuntime(chatID modeluuid.UUID, bindings []coremodel.ChatComponent, fingerprint string) (*ChatRuntime, error) {
	messageCommands, err := b.components.CommandEngineForBindings(bindings, commandengine.SourceMessage)
	if err != nil {
		return nil, err
	}
	agentCommands, err := b.components.CommandEngineForBindings(bindings, commandengine.SourceHostbridge)
	if err != nil {
		return nil, err
	}

	components := b.componentsForBindings(bindings)
	return &ChatRuntime{
		ChatID:          chatID,
		Components:      components,
		MessageCommands: messageCommands,
		AgentCommands:   agentCommands,
		Agents:          componentCapabilities[component.Agent](components),
		Relays:          componentCapabilities[component.OutboundRelay](components),
		Fingerprint:     fingerprint,
	}, nil
}

func (b *Broker) componentsForBindings(bindings []coremodel.ChatComponent) []component.Component {
	if b == nil || b.components == nil {
		return nil
	}
	var out []component.Component
	for _, candidate := range b.components.Components() {
		if matchesAnyBinding(candidate, bindings) {
			out = append(out, candidate)
		}
	}
	return out
}

func componentCapabilities[T any](components []component.Component) []T {
	var out []T
	for _, candidate := range components {
		capability, ok := candidate.(T)
		if ok {
			out = append(out, capability)
		}
	}
	return out
}

func chatRuntimeFingerprint(bindings []coremodel.ChatComponent) string {
	if len(bindings) == 0 {
		return ""
	}
	keys := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		keys = append(keys, chatComponentKey(binding.ComponentType, binding.ProfileName))
	}
	sort.Strings(keys)
	return strings.Join(keys, "\n")
}
