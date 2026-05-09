package broker

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	component "github.com/bartdeboer/ctgbot/internal/component"
	componentbroker "github.com/bartdeboer/ctgbot/internal/component/broker"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
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
	Chat             coremodel.Chat
	Workspace        string
	RuntimeWorkspace string
	Bindings         []coremodel.ChatComponent
	Components       []*component.Loaded
	Agents           []AgentBinding
	Relays           []RelayBinding
	MessageCommands  *commandengine.Engine
	AgentCommands    *commandengine.Engine
	Homes            map[modeluuid.UUID]runtimepkg.Home
}

type AgentBinding struct {
	ComponentID modeluuid.UUID
	Agent       component.Agent
	Completion  component.CompletionAgent
}

type RelayBinding struct {
	ComponentID modeluuid.UUID
	Binding     coremodel.ChatComponent
	Relay       component.OutboundRelay
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

	decision, err := b.checkInboundFirewall(ctx, event)
	if err != nil {
		return EventOutcome{}, err
	}
	if !decision.Allowed {
		actor := event.Payload.ResolvedActor()
		b.logf(
			"inbound dropped component=%s external_chat=%q external_thread=%q reason=%s actor_id=%q actor_label=%q chat_label=%q preview=%q",
			event.ComponentID,
			externalChatID,
			strings.TrimSpace(event.Payload.ProviderThreadID),
			decision.Reason,
			strings.TrimSpace(actor.ID),
			strings.TrimSpace(actor.Label),
			strings.TrimSpace(event.Payload.ChatLabel),
			inboundPreview(event.Payload.Text.Text),
		)
		b.maybeHandleInboundFirewallInit(ctx, event, decision)
		return EventOutcome{Dropped: true}, nil
	}
	sourceBinding := decision.SourceBinding
	chat := decision.Chat

	thread, err := b.Mapper.EnsureThread(ctx, *sourceBinding, strings.TrimSpace(event.Payload.ProviderThreadID))
	if err != nil {
		return EventOutcome{}, err
	}
	var runtime *ChatRuntime
	failConversation := func(inbound *coremodel.ThreadMessage, outbound []coremodel.ThreadMessage, turnErr error) (EventOutcome, error) {
		text := conversationErrorText(turnErr)
		if text == "" {
			return EventOutcome{Inbound: inbound, Outbound: outbound}, nil
		}
		message, relayErr := b.relaySystemMessage(ctx, runtime, *chat, *thread, text)
		if relayErr != nil {
			b.logf("inbound error relay failed chat=%s thread=%s err=%v", chat.ID, thread.ID, relayErr)
			return EventOutcome{Inbound: inbound, Outbound: outbound}, nil
		}
		if message != nil {
			outbound = append(outbound, *message)
		}
		return EventOutcome{Inbound: inbound, Outbound: outbound}, nil
	}

	rawText := strings.TrimSpace(event.Payload.Text.Text)
	if _, ok := commandArgv(rawText); ok {
		runtime, err = b.runtimeForChat(ctx, *chat)
		if err != nil {
			return failConversation(nil, nil, err)
		}
		handled, commandOutbound, err := b.tryHandleMessageCommand(
			ctx,
			event,
			*chat,
			*thread,
			runtime,
		)
		if err != nil {
			return failConversation(nil, commandOutbound, err)
		}
		if handled {
			return EventOutcome{Outbound: commandOutbound}, nil
		}
	}

	var savedPaths []string
	if len(event.Payload.Attachments) > 0 {
		if runtime == nil {
			runtime, err = b.runtimeForChat(ctx, *chat)
			if err != nil {
				return failConversation(nil, nil, err)
			}
		}
		workspacePath, resolveErr := b.Resolver.ResolveChatWorkspace(ctx, *chat)
		if resolveErr != nil {
			return failConversation(nil, nil, resolveErr)
		}
		savedPaths, err = materializeIncomingAttachments(workspacePath, runtime.RuntimeWorkspace, event.Payload.Attachments)
		if err != nil {
			return failConversation(nil, nil, err)
		}
	}

	inbound, err := b.storeInboundMessage(ctx, *chat, *thread, event)
	if err != nil {
		return failConversation(nil, nil, err)
	}
	if rawText == "" && len(savedPaths) > 0 {
		message, relayErr := b.relaySystemMessage(ctx, runtime, *chat, *thread, uploadSavedMessage(savedPaths))
		if relayErr != nil {
			return failConversation(inbound, nil, relayErr)
		}
		outbound := []coremodel.ThreadMessage{}
		if message != nil {
			outbound = append(outbound, *message)
		}
		return EventOutcome{Inbound: inbound, Outbound: outbound}, nil
	}
	turnInbound := *inbound
	if len(savedPaths) > 0 {
		turnInbound.Text = injectFilesIntoPrompt(savedPaths, rawText)
	}

	if runtime == nil {
		runtime, err = b.runtimeForChat(ctx, *chat)
		if err != nil {
			return failConversation(inbound, nil, err)
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
		final, err := b.runAgentTurn(ctx, agentBinding, *chat, *thread, turnInbound, turnRuntime)
		outbound = append(outbound, turnRuntime.outputs...)
		turnRuntime.outputs = nil
		if err != nil {
			return failConversation(inbound, outbound, err)
		}
		if final == nil || strings.TrimSpace(final.Text) == "" {
			continue
		}
		if strings.TrimSpace(final.Text) == turnRuntime.LastText() {
			continue
		}
		message, err := b.storeAndRelayMessage(ctx, runtime, *chat, *thread, *final, agentType(agentBinding))
		if err != nil {
			return failConversation(inbound, outbound, err)
		}
		outbound = append(outbound, *message)
	}

	return EventOutcome{Inbound: inbound, Outbound: outbound}, nil
}

func (b *Broker) runAgentTurn(
	ctx context.Context,
	agentBinding AgentBinding,
	chat coremodel.Chat,
	thread coremodel.Thread,
	inbound coremodel.ThreadMessage,
	turnRuntime *agentTurnRuntime,
) (*coremodel.ThreadMessage, error) {
	if agentBinding.Completion != nil {
		prompt, err := b.completionPrompt(ctx, thread.ID, inbound)
		if err != nil {
			return nil, err
		}
		result, err := agentBinding.Completion.HandleCompletion(ctx, component.CompletionRequest{
			Chat:    chat,
			Thread:  thread,
			Prompt:  prompt,
			Runtime: turnRuntime,
		})
		if err != nil || result == nil {
			return nil, err
		}
		return result.Final, nil
	}
	if agentBinding.Agent == nil {
		return nil, nil
	}
	result, err := agentBinding.Agent.HandleTurn(ctx, component.Turn{
		Chat:    chat,
		Thread:  thread,
		Inbound: inbound,
		Runtime: turnRuntime,
	})
	if err != nil || result == nil {
		return nil, err
	}
	return result.Final, nil
}

func agentType(binding AgentBinding) string {
	if binding.Completion != nil {
		return binding.Completion.Type()
	}
	if binding.Agent != nil {
		return binding.Agent.Type()
	}
	return ""
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
