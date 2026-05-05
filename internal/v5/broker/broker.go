package broker

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
	component "github.com/bartdeboer/ctgbot/internal/v5/component"
	componentbroker "github.com/bartdeboer/ctgbot/internal/v5/component/broker"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
)

type InstanceResolver interface {
	ResolveComponent(ctx context.Context, componentID modeluuid.UUID) (*component.Loaded, error)
	ResolveChatWorkspace(ctx context.Context, chat coremodel.Chat) (string, error)
	ResolveChatHostbridgeAllowedCommands(ctx context.Context, chat coremodel.Chat) (map[string]hostbridgeserver.AllowedCommand, error)
}

type Broker struct {
	Storage  repository.Storage
	Resolver InstanceResolver
	Mapper   ThreadComponentMapper
	Logf     func(format string, args ...any)
}

type EventOutcome struct {
	Dropped  bool
	Inbound  *coremodel.ThreadMessage
	Outbound []coremodel.ThreadMessage
}

type ChatRuntime struct {
	Chat            coremodel.Chat
	Workspace       string
	Bindings        []coremodel.ChatComponent
	Components      []*component.Loaded
	Agents          []AgentBinding
	Relays          []component.OutboundRelay
	MessageCommands *commandengine.Engine
	AgentCommands   *commandengine.Engine
	Homes           map[modeluuid.UUID]v5runtime.Home
}

type AgentBinding struct {
	ComponentID modeluuid.UUID
	Agent       component.Agent
}

func New(storage repository.Storage, resolver InstanceResolver, logf func(format string, args ...any)) *Broker {
	return &Broker{
		Storage:  storage,
		Resolver: resolver,
		Mapper:   NewThreadComponentMapper(storage),
		Logf:     logf,
	}
}

func (b *Broker) HandleInbound(ctx context.Context, event component.InboundEvent) (EventOutcome, error) {
	if err := b.ensureReady(); err != nil {
		return EventOutcome{}, err
	}
	if event.ComponentID.IsNull() {
		return EventOutcome{}, fmt.Errorf("missing inbound component id")
	}
	externalChatID := strings.TrimSpace(event.Payload.ProviderChatID)
	if externalChatID == "" {
		return EventOutcome{}, fmt.Errorf("missing inbound provider chat id")
	}

	sourceBinding, err := b.Storage.ChatComponents().FindByComponentRoleAndExternalChatID(ctx, event.ComponentID, coremodel.ChatComponentRoleSource, externalChatID)
	if err != nil {
		return EventOutcome{}, err
	}
	if sourceBinding == nil {
		b.logf("v5 inbound dropped component=%s external_chat=%q reason=no-source-binding", event.ComponentID, externalChatID)
		return EventOutcome{Dropped: true}, nil
	}

	chat, err := b.Storage.Chats().GetByID(ctx, sourceBinding.ChatID)
	if err != nil {
		return EventOutcome{}, err
	}
	if chat == nil || !chat.Enabled {
		return EventOutcome{Dropped: true}, nil
	}

	thread, err := b.Mapper.EnsureThread(ctx, *sourceBinding, strings.TrimSpace(event.Payload.ProviderThreadID))
	if err != nil {
		return EventOutcome{}, err
	}

	var runtime *ChatRuntime
	if _, ok := commandArgv(event.Payload.Text.Text); ok {
		runtime, err = b.runtimeForChat(ctx, *chat)
		if err != nil {
			return EventOutcome{}, err
		}
		handled, commandOutbound, err := b.tryHandleMessageCommand(
			ctx,
			event,
			*chat,
			*thread,
			runtime,
		)
		if err != nil {
			return EventOutcome{Outbound: commandOutbound}, err
		}
		if handled {
			return EventOutcome{Outbound: commandOutbound}, nil
		}
	}

	inbound, err := b.appendInbound(ctx, *chat, *thread, event)
	if err != nil {
		return EventOutcome{}, err
	}

	if runtime == nil {
		runtime, err = b.runtimeForChat(ctx, *chat)
		if err != nil {
			return EventOutcome{Inbound: inbound}, err
		}
	}

	turnRuntime := &agentTurnRuntime{
		ctx:     ctx,
		broker:  b,
		runtime: runtime,
		chat:    *chat,
		thread:  *thread,
	}

	var outbound []coremodel.ThreadMessage
	for _, agentBinding := range runtime.Agents {
		turnRuntime.componentID = agentBinding.ComponentID
		turnRuntime.lastText = ""
		result, err := agentBinding.Agent.HandleTurn(ctx, component.Turn{
			Chat:    *chat,
			Thread:  *thread,
			Inbound: *inbound,
			Runtime: turnRuntime,
		})
		outbound = append(outbound, turnRuntime.outputs...)
		turnRuntime.outputs = nil
		if err != nil {
			return EventOutcome{Inbound: inbound, Outbound: outbound}, err
		}
		if result == nil || result.Final == nil || strings.TrimSpace(result.Final.Text) == "" {
			continue
		}
		if strings.TrimSpace(result.Final.Text) == turnRuntime.LastText() {
			continue
		}
		message, err := b.appendAndRelayMessage(ctx, runtime, *chat, *thread, *result.Final, agentBinding.Agent.Type())
		if err != nil {
			return EventOutcome{Inbound: inbound, Outbound: outbound}, err
		}
		outbound = append(outbound, *message)
	}

	return EventOutcome{Inbound: inbound, Outbound: outbound}, nil
}

func (b *Broker) ensureReady() error {
	if b == nil || b.Storage == nil {
		return fmt.Errorf("missing broker storage")
	}
	if b.Resolver == nil {
		return fmt.Errorf("missing component resolver")
	}
	if b.Mapper == nil {
		return fmt.Errorf("missing thread component mapper")
	}
	return nil
}

func (b *Broker) logf(format string, args ...any) {
	if b != nil && b.Logf != nil {
		b.Logf(format, args...)
		return
	}
	log.Printf(format, args...)
}

func (b *Broker) RunHostbridgeCommand(ctx context.Context, req commandengine.Request, cmd schemacommands.RunCommand) (commandengine.Result, error) {
	allowed := hostbridgeserver.DefaultAllowedCommands()
	if b != nil && b.Storage != nil && b.Resolver != nil && !req.Context.ChatID.IsNull() {
		chat, err := b.Storage.Chats().GetByID(ctx, req.Context.ChatID)
		if err != nil {
			return commandengine.Result{}, err
		}
		if chat != nil {
			extra, err := b.Resolver.ResolveChatHostbridgeAllowedCommands(ctx, *chat)
			if err != nil {
				return commandengine.Result{}, err
			}
			allowed = hostbridgeserver.MergeNamedAllowedCommands(extra)
		}
	}
	runner := &hostbridgeserver.RunCommandRunner{
		ResolveAllowed:    hostbridgeserver.StaticAllowedCommandResolver(allowed),
		DefaultTimeoutSec: 30,
	}
	return runner.RunCommand(ctx, req, cmd)
}

func (b *Broker) MessageHelp(ctx context.Context, chatID modeluuid.UUID) (string, error) {
	if b == nil {
		return "", fmt.Errorf("missing broker")
	}
	if chatID.IsNull() {
		return "", fmt.Errorf("missing chat id")
	}
	chat, err := b.Storage.Chats().GetByID(ctx, chatID)
	if err != nil {
		return "", err
	}
	if chat == nil {
		return "", fmt.Errorf("chat not found: %s", chatID)
	}
	runtime, err := b.runtimeForChat(ctx, *chat)
	if err != nil {
		return "", err
	}
	if runtime == nil || runtime.MessageCommands == nil {
		return componentbroker.FormatHelp(nil), nil
	}
	return componentbroker.FormatHelp(runtime.MessageCommands.Definitions()), nil
}
