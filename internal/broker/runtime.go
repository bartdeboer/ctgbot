package broker

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	component "github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func (b *Broker) runtimeForChat(ctx context.Context, chat coremodel.Chat) (*ChatRuntime, error) {

	spec, err := b.runtimeSpec(ctx, chat)
	if err != nil {
		return nil, err
	}
	workspace := spec.Workspace
	bindings := spec.Bindings
	runCommands, err := b.runCommandsForChat(ctx, chat)
	if err != nil {
		return nil, err
	}

	profiles := map[modeluuid.UUID]runtimepkg.Profile{}
	var (
		components       []*component.Loaded
		agents           []AgentBinding
		relays           []RelayBinding
		boundSurfaces    []commandset.BoundSurface
		globalSurfaces   []component.CommandSurface
		runtimeWorkspace string
	)
	commandSurfaceKeys := map[string]struct{}{}
	addBoundSurface := func(key string, surface component.CommandSurface, loaded *component.Loaded) {
		if surface == nil || loaded == nil {
			return
		}
		if _, seen := commandSurfaceKeys[key]; seen {
			return
		}
		commandSurfaceKeys[key] = struct{}{}
		boundSurfaces = append(boundSurfaces, commandset.BoundSurface{
			Surface:       surface,
			ComponentRef:  loaded.Registration.Ref(),
			ComponentType: loaded.Registration.Type,
		})
	}

	for _, binding := range bindings {
		instance := spec.Loaded[binding.ComponentID]
		if instance == nil {
			continue
		}
		if receiver, ok := instance.Component.(component.ChatPayloadSenderReceiver); ok {
			receiver.SetChatPayloadSender(b)
		}
		if receiver, ok := instance.Component.(component.SearchMessageSourceReceiver); ok {
			receiver.SetSearchMessageSource(b.App)
		}
		if _, seen := profiles[binding.ComponentID]; !seen {
			profiles[binding.ComponentID] = instance.Profile
			components = append(components, instance)
		}
		if surface, ok := instance.Component.(component.CommandSurface); ok {
			addBoundSurface("command:"+binding.ComponentID.String(), surface, instance)
		}
		switch binding.Role {
		case coremodel.ChatComponentRoleAgent:
			if completionImpl, ok := instance.Component.(component.CompletionEngine); ok {
				agents = append(agents, AgentBinding{
					ComponentID: binding.ComponentID,
					Completion:  completionImpl,
				})
				if runtimeWorkspace == "" && instance.Runtime != nil {
					runtimeWorkspace = strings.TrimSpace(instance.Runtime.RuntimeWorkspacePath(workspace))
				}
			} else if turnHandler, ok := instance.Component.(component.TurnHandler); ok {
				agents = append(agents, AgentBinding{
					ComponentID: binding.ComponentID,
					TurnHandler: turnHandler,
				})
				if runtimeWorkspace == "" && instance.Runtime != nil {
					runtimeWorkspace = strings.TrimSpace(instance.Runtime.RuntimeWorkspacePath(workspace))
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
		}
	}
	updateFeeds := activeUpdateFeeds(components)
	for _, instance := range components {
		if receiver, ok := instance.Component.(component.UpdateFeedReceiver); ok {
			receiver.SetUpdateFeeds(updateFeeds)
		}
	}

	if runtimeWorkspace == "" {
		runtimeWorkspace = workspace
	}

	globalSurfaces, err = b.App.CommandSurfaces(ctx, chat, b, b)
	if err != nil {
		return nil, err
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
		RunCommands:      runCommands,
		Profiles:         profiles,
	}, nil
}

func (b *Broker) runCommandsForChat(ctx context.Context, chat coremodel.Chat) (map[string]hostbridgeserver.AllowedCommand, error) {
	allowed := hostbridgeserver.DefaultAllowedCommands()
	if b == nil || b.App == nil {
		return allowed, nil
	}
	extra, err := b.App.ResolveChatHostbridgeAllowedCommands(ctx, chat)
	if err != nil {
		return nil, err
	}
	return hostbridgeserver.MergeNamedAllowedCommands(extra), nil
}

func (r *ChatRuntime) RunHostbridgeAllowedCommand(ctx context.Context, req commandengine.Request, cmd schemacommands.RunCommand) (commandengine.Result, error) {
	allowed := hostbridgeserver.DefaultAllowedCommands()
	if r != nil && r.RunCommands != nil {
		allowed = r.RunCommands
	}
	runner := &hostbridgeserver.RunCommandRunner{
		ResolveAllowed:    hostbridgeserver.StaticAllowedCommandResolver(allowed),
		DefaultTimeoutSec: 30,
	}
	return runner.RunCommand(ctx, req, cmd)
}

type runtimeSpec struct {
	Workspace string
	Bindings  []coremodel.ChatComponent
	Loaded    map[modeluuid.UUID]*component.Loaded
}

func (b *Broker) runtimeSpec(ctx context.Context, chat coremodel.Chat) (runtimeSpec, error) {
	workspace, err := b.App.ResolveChatWorkspace(ctx, chat)
	if err != nil {
		return runtimeSpec{}, err
	}
	bindings, err := b.App.EnabledChatComponents(ctx, chat.ID)
	if err != nil {
		return runtimeSpec{}, err
	}
	loaded := make(map[modeluuid.UUID]*component.Loaded)
	for _, binding := range bindings {
		if _, ok := loaded[binding.ComponentID]; ok {
			continue
		}
		instance, err := b.App.ResolveComponent(ctx, binding.ComponentID)
		if err != nil {
			return runtimeSpec{}, err
		}
		loaded[binding.ComponentID] = instance
	}
	return runtimeSpec{Workspace: workspace, Bindings: bindings, Loaded: loaded}, nil
}

type agentTurnRuntime struct {
	ctx                   context.Context
	broker                *Broker
	runtime               *ChatRuntime
	chat                  coremodel.Chat
	thread                coremodel.Thread
	componentID           modeluuid.UUID
	outputs               []coremodel.ThreadMessage
	lastText              string
	voiceInput            bool
	detectedInputLanguage string
	voiceOutput           bool
	voiceLanguage         string
	voiceName             string
	voiceModel            string
	voiceDeviceTarget     string
	inputFiles            []turnInputFile
}

type turnInputFile struct {
	Path        string
	Kind        string
	Filename    string
	ContentType string
	Temporary   bool
}

func (r *agentTurnRuntime) Commands() commandengine.CommandExecutor {
	if r == nil || r.runtime == nil {
		return nil
	}
	return turnCommandExecutor{turn: r, next: r.runtime.AgentCommands}
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
	instructions.HostbridgeCommandNames = hostbridgeserver.AllowedCommandUsages(r.runtime.RunCommands)
	sort.Strings(instructions.HostbridgeCommandNames)
	instructions.HostbridgeControlCommands = hostbridgeControlCommands(r.runtime)
	instructions.HostbridgeFamilyDescriptions = hostbridgeFamilyDescriptions(r.runtime)
	return instructions
}

func hostbridgeControlCommands(runtime *ChatRuntime) []string {
	if runtime == nil || runtime.AgentCommands == nil {
		return nil
	}
	patterns := commandset.InstructionRoutePatterns(
		runtime.AgentCommands.Definitions(),
		coremodel.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
	)
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		if strings.TrimSpace(pattern) == "" {
			continue
		}
		out = append(out, "hostbridge "+pattern)
	}
	return out
}

func hostbridgeFamilyDescriptions(runtime *ChatRuntime) map[string]string {
	if runtime == nil || runtime.AgentCommands == nil {
		return nil
	}
	return commandset.InstructionFamilyDescriptions(
		runtime.AgentCommands.Descriptions(),
		coremodel.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
	)
}

func (r *agentTurnRuntime) Send(ctx context.Context, payload message.OutboundPayload) error {
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

func (r *agentTurnRuntime) StartChatAction(ctx context.Context, action message.ChatAction) (func(), error) {
	if r == nil || r.runtime == nil || r.broker == nil {
		return func() {}, nil
	}
	var stops []func()
	for _, relayBinding := range r.runtime.Relays {
		target, ok, err := r.broker.App.RelayTarget(ctx, r.thread.ID, relayBinding.Binding)
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

func (r *agentTurnRuntime) ComponentProfile(componentID modeluuid.UUID) (runtimepkg.Profile, bool) {
	if r == nil || r.runtime == nil {
		return runtimepkg.Profile{}, false
	}
	profile, ok := r.runtime.Profiles[componentID]
	return profile, ok
}

func (r *agentTurnRuntime) ComponentThreadID(componentID modeluuid.UUID) (string, bool, error) {
	if r == nil || r.broker == nil {
		return "", false, fmt.Errorf("missing turn runtime")
	}
	return r.broker.App.ComponentThreadID(r.context(), r.thread.ID, componentID)
}

func (r *agentTurnRuntime) BindComponentThreadID(componentID modeluuid.UUID, componentThreadID string) error {
	if r == nil || r.broker == nil {
		return fmt.Errorf("missing turn runtime")
	}
	return r.broker.App.BindComponentThreadID(r.context(), r.thread.ID, componentID, componentThreadID)
}

func (r *agentTurnRuntime) context() context.Context {
	if r != nil && r.ctx != nil {
		return r.ctx
	}
	return context.Background()
}

func activeUpdateFeeds(components []*component.Loaded) []component.UpdateFeed {
	var feeds []component.UpdateFeed
	for _, instance := range components {
		if instance == nil || instance.Component == nil {
			continue
		}
		if feed, ok := instance.Component.(component.UpdateFeed); ok {
			feeds = append(feeds, feed)
		}
	}
	return feeds
}
