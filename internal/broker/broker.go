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
	"github.com/bartdeboer/ctgbot/internal/inbound"
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

type App interface {
	Repository() repository.Storage
	InstanceResolver
	InboundFilters() []inbound.Filter
}

type Broker struct {
	App            App
	Storage        repository.Storage
	Resolver       InstanceResolver
	Mapper         ThreadComponentMapper
	Turns          *ThreadTurnGate
	Logf           func(format string, args ...any)
	InboundFilters []inbound.Filter
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
	Completion  component.CompletionProvider
}

type RelayBinding struct {
	ComponentID modeluuid.UUID
	Binding     coremodel.ChatComponent
	Relay       component.OutboundRelay
}

func New(app App, logf func(format string, args ...any)) *Broker {
	if app == nil {
		return NewWithDeps(nil, nil, logf)
	}
	broker := NewWithDeps(app.Repository(), app, logf, app.InboundFilters()...)
	broker.App = app
	return broker
}

func NewWithDeps(storage repository.Storage, resolver InstanceResolver, logf func(format string, args ...any), filters ...inbound.Filter) *Broker {
	return &Broker{
		Storage:        storage,
		Resolver:       resolver,
		Mapper:         NewThreadComponentMapper(storage),
		Turns:          NewThreadTurnGate(),
		Logf:           logf,
		InboundFilters: inboundFilters(storage, filters...),
	}
}

func (b *Broker) repository() repository.Storage {
	if b == nil {
		return nil
	}
	if b.App != nil {
		if storage := b.App.Repository(); storage != nil {
			return storage
		}
	}
	return b.Storage
}

func (b *Broker) resolver() InstanceResolver {
	if b == nil {
		return nil
	}
	if b.App != nil {
		return b.App
	}
	return b.Resolver
}

func (b *Broker) HandleInbound(ctx context.Context, event component.InboundEvent) (EventOutcome, error) {
	if err := b.ensureReady(); err != nil {
		return EventOutcome{}, err
	}
	if event.ComponentID.IsNull() {
		return EventOutcome{}, fmt.Errorf("missing inbound component id")
	}

	envelope, filterResult, err := b.filterInbound(ctx, inbound.Envelope{Event: event})
	if err != nil {
		return EventOutcome{}, err
	}
	if filterResult.Drop {
		dropEvent := filterResult.Envelope.Event
		if dropEvent.ComponentID.IsNull() {
			dropEvent = event
		}
		actor := dropEvent.Payload.ResolvedActor()
		details := strings.Join(filterResult.Details, " ")
		b.logf(
			"inbound dropped component=%s external_chat=%q external_thread=%q reason=%s actor_id=%q actor_label=%q chat_label=%q preview=%q details=%q",
			dropEvent.ComponentID,
			strings.TrimSpace(dropEvent.Payload.ProviderChatID),
			strings.TrimSpace(dropEvent.Payload.ProviderThreadID),
			filterResult.Reason,
			strings.TrimSpace(actor.ID),
			strings.TrimSpace(actor.Label),
			strings.TrimSpace(dropEvent.Payload.ChatLabel),
			inboundPreview(dropEvent.Payload.Text.Text),
			details,
		)
		b.maybeHandleInboundFirewallInit(ctx, filterResult)
		return EventOutcome{Dropped: true}, nil
	}
	sourceBinding := envelope.SourceBinding
	chat := envelope.Chat
	if sourceBinding == nil || chat == nil {
		return EventOutcome{}, fmt.Errorf("inbound filters did not resolve chat routing")
	}
	routedEvent := envelope.Event

	thread, err := b.Mapper.EnsureThread(ctx, *sourceBinding, strings.TrimSpace(routedEvent.Payload.ProviderThreadID))
	if err != nil {
		return EventOutcome{}, err
	}
	delivery, err := b.HandleResolvedInbound(ctx, component.ResolvedInbound{
		Chat:        *chat,
		Thread:      *thread,
		ComponentID: routedEvent.ComponentID,
		ExternalID:  strings.TrimSpace(routedEvent.ExternalID),
		Payload:     routedEvent.Payload,
	})
	if err != nil {
		return EventOutcome{
			Inbound:  delivery.Inbound,
			Outbound: delivery.Outbound,
		}, err
	}
	return EventOutcome{
		Inbound:  delivery.Inbound,
		Outbound: delivery.Outbound,
	}, nil
}

// HandleResolvedInbound runs the common inbound turn path when chat/thread
// routing is already known.
func (b *Broker) HandleResolvedInbound(ctx context.Context, inbound component.ResolvedInbound) (component.DeliveryResult, error) {
	if err := b.ensureReady(); err != nil {
		return component.DeliveryResult{}, err
	}
	if inbound.Chat.ID.IsNull() {
		return component.DeliveryResult{}, fmt.Errorf("missing inbound chat id")
	}
	if inbound.Thread.ID.IsNull() {
		return component.DeliveryResult{}, fmt.Errorf("missing inbound thread id")
	}

	var runtime *ChatRuntime
	failConversation := func(result component.DeliveryResult, turnErr error) (component.DeliveryResult, error) {
		text := conversationErrorText(turnErr)
		if text == "" {
			return result, nil
		}
		message, relayErr := b.relaySystemMessage(ctx, runtime, inbound.Chat, inbound.Thread, text)
		if relayErr != nil {
			b.logf("inbound error relay failed chat=%s thread=%s err=%v", inbound.Chat.ID, inbound.Thread.ID, relayErr)
			return result, nil
		}
		if message != nil {
			result.Outbound = append(result.Outbound, *message)
		}
		return result, nil
	}

	rawText := strings.TrimSpace(inbound.Payload.Text.Text)
	if _, ok := commandArgv(rawText); ok {
		var err error
		runtime, err = b.runtimeForChat(ctx, inbound.Chat)
		if err != nil {
			return failConversation(component.DeliveryResult{}, err)
		}
		handled, commandOutbound, err := b.tryHandleMessageCommand(ctx, inbound, inbound.Chat, inbound.Thread, runtime)
		if err != nil {
			return failConversation(component.DeliveryResult{Outbound: commandOutbound}, err)
		}
		if handled {
			return component.DeliveryResult{Outbound: commandOutbound}, nil
		}
	}

	var outcome EventOutcome
	if err := b.Turns.Run(ctx, inbound.Thread.ID, func() error {
		var runErr error
		outcome, runErr = b.handleResolvedInboundTurn(ctx, inbound, runtime)
		return runErr
	}); err != nil {
		return component.DeliveryResult{
			Inbound:  outcome.Inbound,
			Outbound: outcome.Outbound,
		}, err
	}
	return component.DeliveryResult{
		Inbound:  outcome.Inbound,
		Outbound: outcome.Outbound,
	}, nil
}

func (b *Broker) handleResolvedInboundTurn(
	ctx context.Context,
	inbound component.ResolvedInbound,
	runtime *ChatRuntime,
) (EventOutcome, error) {
	var err error
	chat := inbound.Chat
	thread := inbound.Thread
	rawText := strings.TrimSpace(inbound.Payload.Text.Text)
	failConversation := func(inbound *coremodel.ThreadMessage, outbound []coremodel.ThreadMessage, turnErr error) (EventOutcome, error) {
		text := conversationErrorText(turnErr)
		if text == "" {
			return EventOutcome{Inbound: inbound, Outbound: outbound}, nil
		}
		message, relayErr := b.relaySystemMessage(ctx, runtime, chat, thread, text)
		if relayErr != nil {
			b.logf("inbound error relay failed chat=%s thread=%s err=%v", chat.ID, thread.ID, relayErr)
			return EventOutcome{Inbound: inbound, Outbound: outbound}, nil
		}
		if message != nil {
			outbound = append(outbound, *message)
		}
		return EventOutcome{Inbound: inbound, Outbound: outbound}, nil
	}

	var savedPaths []string
	if len(inbound.Payload.Attachments) > 0 {
		if runtime == nil {
			runtime, err = b.runtimeForChat(ctx, chat)
			if err != nil {
				return failConversation(nil, nil, err)
			}
		}
		workspacePath, resolveErr := b.resolver().ResolveChatWorkspace(ctx, chat)
		if resolveErr != nil {
			return failConversation(nil, nil, resolveErr)
		}
		savedPaths, err = materializeIncomingAttachments(workspacePath, runtime.RuntimeWorkspace, inbound.Payload.Attachments)
		if err != nil {
			return failConversation(nil, nil, err)
		}
	}

	storedInbound, err := b.storeInboundMessage(ctx, inbound)
	if err != nil {
		return failConversation(nil, nil, err)
	}
	if rawText == "" && len(savedPaths) > 0 {
		message, relayErr := b.relaySystemMessage(ctx, runtime, chat, thread, uploadSavedMessage(savedPaths))
		if relayErr != nil {
			return failConversation(storedInbound, nil, relayErr)
		}
		outbound := []coremodel.ThreadMessage{}
		if message != nil {
			outbound = append(outbound, *message)
		}
		return EventOutcome{Inbound: storedInbound, Outbound: outbound}, nil
	}
	turnInbound := *storedInbound
	turnPrompt := rawText
	if len(savedPaths) > 0 {
		turnPrompt = injectFilesIntoPrompt(savedPaths, rawText)
	}
	turnInbound.Text = prepareTurnInbound(inbound, turnPrompt)

	if runtime == nil {
		runtime, err = b.runtimeForChat(ctx, chat)
		if err != nil {
			return failConversation(storedInbound, nil, err)
		}
	}
	outbound, err := b.runStoredThreadTurn(ctx, runtime, chat, thread, turnInbound)
	if err != nil {
		return failConversation(storedInbound, outbound, err)
	}

	return EventOutcome{Inbound: storedInbound, Outbound: outbound}, nil
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
	if b == nil || b.repository() == nil {
		return fmt.Errorf("missing broker storage")
	}
	if b.resolver() == nil {
		return fmt.Errorf("missing component resolver")
	}
	if b.Mapper == nil {
		return fmt.Errorf("missing thread component mapper")
	}
	if b.Turns == nil {
		b.Turns = NewThreadTurnGate()
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
	storage := b.repository()
	resolver := b.resolver()
	if storage != nil && resolver != nil && !req.Context.ChatID.IsNull() {
		chat, err := storage.Chats().GetByID(ctx, req.Context.ChatID)
		if err != nil {
			return commandengine.Result{}, err
		}
		if chat != nil {
			extra, err := resolver.ResolveChatHostbridgeAllowedCommands(ctx, *chat)
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
	chat, err := b.repository().Chats().GetByID(ctx, chatID)
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
