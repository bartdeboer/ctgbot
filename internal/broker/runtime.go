package broker

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	component "github.com/bartdeboer/ctgbot/internal/component"
	brokercomponent "github.com/bartdeboer/ctgbot/internal/component/broker"
	configcomponent "github.com/bartdeboer/ctgbot/internal/component/config"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	v5runtime "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func (b *Broker) runtimeForChat(ctx context.Context, chat coremodel.Chat) (*ChatRuntime, error) {
	workspace, err := b.Resolver.ResolveChatWorkspace(ctx, chat)
	if err != nil {
		return nil, err
	}

	bindings, err := b.Storage.ChatComponents().ListEnabledByChatID(ctx, chat.ID)
	if err != nil {
		return nil, err
	}

	resolved := map[modeluuid.UUID]*component.Loaded{}
	homes := map[modeluuid.UUID]v5runtime.Home{}
	var (
		components       []*component.Loaded
		agents           []AgentBinding
		relays           []RelayBinding
		boundSurfaces    []commandset.BoundSurface
		globalSurfaces   []component.CommandSurface
		runtimeWorkspace string
	)
	commandSurfaceIDs := map[modeluuid.UUID]struct{}{}

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
			if completionImpl, ok := instance.Component.(component.CompletionAgent); ok {
				agents = append(agents, AgentBinding{
					ComponentID: binding.ComponentID,
					Completion:  completionImpl,
				})
				if runtimeWorkspace == "" && instance.Runtime != nil {
					runtimeWorkspace = strings.TrimSpace(instance.Runtime.RuntimeWorkspacePath(workspace))
				}
			} else if agentImpl, ok := instance.Component.(component.Agent); ok {
				agents = append(agents, AgentBinding{
					ComponentID: binding.ComponentID,
					Agent:       agentImpl,
				})
				if runtimeWorkspace == "" && instance.Runtime != nil {
					runtimeWorkspace = strings.TrimSpace(instance.Runtime.RuntimeWorkspacePath(workspace))
				}
			}
			if surface, ok := instance.Component.(component.CommandSurface); ok {
				if _, seen := commandSurfaceIDs[binding.ComponentID]; !seen {
					commandSurfaceIDs[binding.ComponentID] = struct{}{}
					boundSurfaces = append(boundSurfaces, commandset.BoundSurface{
						Surface:       surface,
						ComponentRef:  instance.Registration.Ref(),
						ComponentType: instance.Registration.Type,
					})
				}
			}
		case coremodel.ChatComponentRoleRelay:
			if relay, ok := instance.Component.(component.OutboundRelay); ok {
				relays = append(relays, RelayBinding{
					ComponentID: binding.ComponentID,
					Binding:     binding,
					Relay:       relay,
				})
			}
		case coremodel.ChatComponentRoleCommand:
			if surface, ok := instance.Component.(component.CommandSurface); ok {
				if _, seen := commandSurfaceIDs[binding.ComponentID]; !seen {
					commandSurfaceIDs[binding.ComponentID] = struct{}{}
					boundSurfaces = append(boundSurfaces, commandset.BoundSurface{
						Surface:       surface,
						ComponentRef:  instance.Registration.Ref(),
						ComponentType: instance.Registration.Type,
					})
				}
			}
		}
	}
	if runtimeWorkspace == "" {
		runtimeWorkspace = workspace
	}

	globalSurfaces = append(globalSurfaces, brokercomponent.New(b))
	if provider, ok := b.Resolver.(interface{ AppConfig() *appstate.Config }); ok {
		configSurface, err := configcomponent.New(provider.AppConfig())
		if err != nil {
			return nil, err
		}
		if configSurface != nil {
			globalSurfaces = append(globalSurfaces, configSurface)
		}
	}

	messageCommands, err := commandset.NewBoundEngineForSource(commandengine.SourceMessage, boundSurfaces, globalSurfaces...)
	if err != nil {
		return nil, err
	}
	agentCommands, err := commandset.NewBoundEngineForSource(commandengine.SourceHostbridge, boundSurfaces, globalSurfaces...)
	if err != nil {
		return nil, err
	}

	return &ChatRuntime{
		Chat:             chat,
		Workspace:        workspace,
		RuntimeWorkspace: runtimeWorkspace,
		Bindings:         bindings,
		Components:       components,
		Agents:           agents,
		Relays:           relays,
		MessageCommands:  messageCommands,
		AgentCommands:    agentCommands,
		Homes:            homes,
	}, nil
}

type agentTurnRuntime struct {
	ctx         context.Context
	broker      *Broker
	runtime     *ChatRuntime
	chat        coremodel.Chat
	thread      coremodel.Thread
	componentID modeluuid.UUID
	outputs     []coremodel.ThreadMessage
	lastText    string
}

func (r *agentTurnRuntime) Commands() commandengine.CommandExecutor {
	if r == nil || r.runtime == nil {
		return nil
	}
	return r.runtime.AgentCommands
}

func (r *agentTurnRuntime) Instructions() component.TurnInstructions {
	instructions := component.TurnInstructions{ChatProvider: "Chat"}
	if r == nil || r.runtime == nil {
		return instructions
	}
	for _, loaded := range r.runtime.Components {
		if loaded == nil {
			continue
		}
		if loaded.Registration.Type == "telegram" {
			instructions.ChatProvider = "Telegram"
			instructions.MessagePrefix = "🤖"
			instructions.KeepRepliesConcise = true
			break
		}
	}
	if r.broker != nil && r.broker.Resolver != nil {
		allowed := hostbridgeserver.DefaultAllowedCommands()
		extra, err := r.broker.Resolver.ResolveChatHostbridgeAllowedCommands(r.context(), r.chat)
		if err == nil {
			allowed = hostbridgeserver.MergeNamedAllowedCommands(extra)
		}
		instructions.HostbridgeCommandNames = hostbridgeserver.AllowedCommandNames(allowed)
		sort.Strings(instructions.HostbridgeCommandNames)
	}
	instructions.HostbridgeControlCommands = hostbridgeControlCommands(r.runtime)
	return instructions
}

func hostbridgeControlCommands(runtime *ChatRuntime) []string {
	if runtime == nil || runtime.AgentCommands == nil {
		return nil
	}
	patterns := commandset.CanonicalRoutePatterns(
		runtime.AgentCommands.Definitions(),
		simplerbac.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
	)
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		switch {
		case strings.TrimSpace(pattern) == "":
			continue
		case strings.HasPrefix(pattern, "run "):
			continue
		case strings.HasPrefix(pattern, "sendfile"):
			continue
		case strings.HasPrefix(pattern, "sendstdin"):
			continue
		default:
			out = append(out, "hostbridge "+pattern)
		}
	}
	return out
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
	if text := strings.TrimSpace(payload.Text.Text); text != "" {
		r.lastText = text
	}
	return nil
}

func (r *agentTurnRuntime) LastText() string {
	if r == nil {
		return ""
	}
	return r.lastText
}

func (r *agentTurnRuntime) StartChatAction(ctx context.Context, action messenger.ChatAction) (func(), error) {
	if r == nil || r.runtime == nil || r.broker == nil {
		return func() {}, nil
	}
	var stops []func()
	for _, relayBinding := range r.runtime.Relays {
		target, ok, err := r.broker.Mapper.RelayTarget(ctx, r.thread.ID, relayBinding.Binding)
		if err != nil {
			for _, s := range stops {
				s()
			}
			return nil, err
		}
		if !ok {
			continue
		}
		stop, err := relayBinding.Relay.StartChatAction(ctx, *target, action)
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
	return func() {
		for _, stop := range stops {
			stop()
		}
	}, nil
}

func (r *agentTurnRuntime) WorkspacePath() string {
	if r == nil || r.runtime == nil {
		return ""
	}
	return r.runtime.Workspace
}

func (r *agentTurnRuntime) ComponentHome(componentID modeluuid.UUID) (v5runtime.Home, bool) {
	if r == nil || r.runtime == nil {
		return v5runtime.Home{}, false
	}
	home, ok := r.runtime.Homes[componentID]
	return home, ok
}

func (r *agentTurnRuntime) ComponentThreadID(componentID modeluuid.UUID) (string, bool, error) {
	if r == nil || r.broker == nil {
		return "", false, fmt.Errorf("missing turn runtime")
	}
	return r.broker.Mapper.ComponentThreadID(r.context(), r.thread.ID, componentID)
}

func (r *agentTurnRuntime) BindComponentThreadID(componentID modeluuid.UUID, componentThreadID string) error {
	if r == nil || r.broker == nil {
		return fmt.Errorf("missing turn runtime")
	}
	return r.broker.Mapper.BindComponentThreadID(r.context(), r.thread.ID, componentID, componentThreadID)
}

func (r *agentTurnRuntime) context() context.Context {
	if r != nil && r.ctx != nil {
		return r.ctx
	}
	return context.Background()
}
