package broker

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	v3component "github.com/bartdeboer/ctgbot/internal/v3/component"
	"github.com/bartdeboer/ctgbot/internal/v3/coremodel"
)

func (b *Broker) runtimeForChat(ctx context.Context, chat coremodel.Chat) (*ChatRuntime, error) {
	bindings, err := b.Storage.ChatComponents().ListEnabledByChatID(ctx, chat.ID)
	if err != nil {
		return nil, err
	}

	resolved := map[modeluuid.UUID]*v3component.Instance{}
	homes := map[modeluuid.UUID]v3component.Home{}
	var (
		components []*v3component.Instance
		agents     []AgentBinding
		relays     []v3component.OutboundRelay
		surfaces   []v3component.CommandSurface
	)

	for _, binding := range bindings {
		instance := resolved[binding.ComponentID]
		if instance == nil {
			instance, err = b.Resolver.ResolveComponent(ctx, binding.ComponentID)
			if err != nil {
				return nil, err
			}
			resolved[binding.ComponentID] = instance
			homes[binding.ComponentID] = instance.Home
			components = append(components, instance)
		}
		switch binding.Role {
		case coremodel.ChatComponentRoleAgent:
			if agentImpl, ok := instance.Implementation.(v3component.Agent); ok {
				agents = append(agents, AgentBinding{
					ComponentID: binding.ComponentID,
					Agent:       agentImpl,
				})
			}
		case coremodel.ChatComponentRoleRelay:
			if relay, ok := instance.Implementation.(v3component.OutboundRelay); ok {
				relays = append(relays, relay)
			}
		case coremodel.ChatComponentRoleCommand:
			if surface, ok := instance.Implementation.(v3component.CommandSurface); ok {
				surfaces = append(surfaces, surface)
			}
		}
	}

	commands, err := buildCommandEngine(surfaces)
	if err != nil {
		return nil, err
	}

	return &ChatRuntime{
		Chat:       chat,
		Bindings:   bindings,
		Components: components,
		Agents:     agents,
		Relays:     relays,
		Commands:   commands,
		Homes:      homes,
	}, nil
}

func buildCommandEngine(surfaces []v3component.CommandSurface) (*commandengine.Engine, error) {
	if len(surfaces) == 0 {
		return nil, nil
	}
	var definitions []commandengine.Definition
	registry := commandengine.NewRegistry()
	for _, surface := range surfaces {
		if surface == nil {
			continue
		}
		definitions = append(definitions, surface.CommandDefinitions()...)
		if err := surface.RegisterCommandHandlers(registry); err != nil {
			return nil, err
		}
	}
	router, err := commandengine.NewRouter(definitions, commandengine.SourceHostbridge)
	if err != nil {
		return nil, err
	}
	return commandengine.NewEngine(router, registry), nil
}

type agentTurnRuntime struct {
	broker      *Broker
	runtime     *ChatRuntime
	chat        coremodel.Chat
	thread      coremodel.Thread
	componentID modeluuid.UUID
	outputs     []coremodel.ThreadMessage
}

func (r *agentTurnRuntime) Commands() commandengine.CommandExecutor {
	if r == nil || r.runtime == nil {
		return nil
	}
	return r.runtime.Commands
}

func (r *agentTurnRuntime) Send(ctx context.Context, payload messenger.OutboundPayload) error {
	if r == nil || r.broker == nil || r.runtime == nil {
		return fmt.Errorf("missing turn runtime")
	}
	messages, err := r.broker.deliverPayload(ctx, r.runtime, r.chat, r.thread, payload, r.componentID)
	if err != nil {
		return err
	}
	r.outputs = append(r.outputs, messages...)
	return nil
}

func (r *agentTurnRuntime) StartChatAction(ctx context.Context, action messenger.ChatAction) (func(), error) {
	if r == nil || r.runtime == nil || r.broker == nil {
		return func() {}, nil
	}
	var stops []func()
	targets, err := r.broker.relayTargetsForRuntime(ctx, r.runtime, r.thread)
	if err != nil {
		return nil, err
	}
	for _, relay := range r.runtime.Relays {
		for _, target := range targets {
			stop, err := relay.StartChatAction(ctx, target, action)
			if err != nil {
				for _, s := range stops {
					s()
				}
				return nil, err
			}
			if stop != nil {
				stops = append(stops, stop)
			}
		}
	}
	return func() {
		for _, stop := range stops {
			stop()
		}
	}, nil
}

func (r *agentTurnRuntime) ComponentHome(componentID modeluuid.UUID) (v3component.Home, bool) {
	if r == nil || r.runtime == nil {
		return v3component.Home{}, false
	}
	home, ok := r.runtime.Homes[componentID]
	return home, ok
}
