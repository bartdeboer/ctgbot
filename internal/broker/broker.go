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
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

type App interface {
	AdmitInbound(ctx context.Context, event component.InboundEvent) (inbound.Admission, error)
	Chat(ctx context.Context, chatID modeluuid.UUID) (*coremodel.Chat, error)
	Thread(ctx context.Context, threadID modeluuid.UUID) (*coremodel.Thread, error)
	ThreadMessages(ctx context.Context, threadID modeluuid.UUID) ([]coremodel.ThreadMessage, error)
	ForEachMessage(ctx context.Context, scope component.MessageScope, visit component.MessageVisitor) error
	EnabledChatComponents(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.ChatComponent, error)
	EnabledInboundSources(ctx context.Context) ([]component.InboundSource, error)
	CommandSurfaces(ctx context.Context, chat coremodel.Chat, inbound component.ResolvedInboundQueuer, actions componentbroker.Actions) ([]component.CommandSurface, error)
	EnsureThread(ctx context.Context, binding coremodel.ChatComponent, componentThreadID string) (*coremodel.Thread, error)
	ComponentThreadID(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) (string, bool, error)
	BindComponentThreadID(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID, componentThreadID string) error
	RelayTarget(ctx context.Context, threadID modeluuid.UUID, binding coremodel.ChatComponent) (*message.ChatTarget, bool, error)
	StoreInboundMessage(ctx context.Context, inbound component.ResolvedInbound) (*coremodel.ThreadMessage, error)
	StoreOutboundMessage(ctx context.Context, message *coremodel.ThreadMessage, attachments []message.Media) error
	DropEvent(ctx context.Context, rejection *inbound.Rejection) (*coremodel.DroppedEvent, error)
	DropNoticeID(ctx context.Context, drop *coremodel.DroppedEvent) string
	ResolveDroppedEventID(ctx context.Context, ref string) (modeluuid.UUID, error)
	DroppedEvent(ctx context.Context, id modeluuid.UUID) (*coremodel.DroppedEvent, error)
	ListDroppedEvents(ctx context.Context, limit int) ([]coremodel.DroppedEvent, error)
	SaveDroppedEvent(ctx context.Context, drop *coremodel.DroppedEvent) error
	ResolveComponent(ctx context.Context, componentID modeluuid.UUID) (*component.Loaded, error)
	ResolveChatWorkspace(ctx context.Context, chat coremodel.Chat) (string, error)
	ResolveChatHostbridgeAllowedCommands(ctx context.Context, chat coremodel.Chat) (map[string]hostbridgeserver.AllowedCommand, error)
}

type Broker struct {
	App   App
	Turns *ThreadTurnGate
	Logf  func(format string, args ...any)
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
	broker := &Broker{
		App:   app,
		Turns: NewThreadTurnGate(),
		Logf:  logf,
	}
	return broker
}

type inboundRouteOptions struct {
	bypassEventFilters bool
}

func (b *Broker) HandleInbound(ctx context.Context, event component.InboundEvent) (EventOutcome, error) {
	return b.handleInbound(ctx, event, inboundRouteOptions{})
}

func (b *Broker) handleInbound(ctx context.Context, event component.InboundEvent, opts inboundRouteOptions) (EventOutcome, error) {
	if err := b.ensureReady(); err != nil {
		return EventOutcome{}, err
	}
	if event.ComponentID.IsNull() {
		return EventOutcome{}, fmt.Errorf("missing inbound component id")
	}

	admission, err := b.admitInbound(ctx, event)
	if err != nil {
		return EventOutcome{}, err
	}
	if admission.Rejected != nil {
		b.handleInboundRejection(ctx, admission.Rejected)
		return EventOutcome{Dropped: true}, nil
	}

	routedEvent := event
	if !opts.bypassEventFilters {
		filterResult, err := inbound.NewFilterChain(admission.Filters).Run(ctx, inbound.ChannelEvent{
			Channel: admission.Channel,
			Event:   event,
		})
		if err != nil {
			return EventOutcome{}, err
		}
		if filterResult.Action == inbound.FilterActionDrop || filterResult.Action == inbound.FilterActionQuarantine {
			b.handleInboundRejection(ctx, inbound.RejectionFromFilter(admission.Channel, event, filterResult))
			return EventOutcome{Dropped: true}, nil
		}
		routedEvent = filterResult.Event
		if routedEvent.ComponentID.IsNull() {
			routedEvent = event
		}
	}

	thread, err := b.App.EnsureThread(ctx, admission.Channel.SourceBinding, strings.TrimSpace(routedEvent.Payload.ProviderThreadID))
	if err != nil {
		return EventOutcome{}, err
	}
	delivery, err := b.HandleResolvedInbound(ctx, component.ResolvedInbound{
		Chat:        admission.Channel.Chat,
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
	voiceMedia, isVoiceTurn := voiceInputAttachment(rawText, inbound.Payload.Attachments)
	if len(inbound.Payload.Attachments) > 0 && !isVoiceTurn {
		if runtime == nil {
			runtime, err = b.runtimeForChat(ctx, chat)
			if err != nil {
				return failConversation(nil, nil, err)
			}
		}
		workspacePath, resolveErr := b.App.ResolveChatWorkspace(ctx, chat)
		if resolveErr != nil {
			return failConversation(nil, nil, resolveErr)
		}
		savedPaths, err = materializeIncomingAttachments(workspacePath, runtime.RuntimeWorkspace, inbound.Payload.Attachments)
		if err != nil {
			return failConversation(nil, nil, err)
		}
	}

	options := turnOptions{Mode: turnModeText}
	turnPrompt := rawText
	if isVoiceTurn {
		options.Mode = turnModeAudio
		if runtime == nil {
			runtime, err = b.runtimeForChat(ctx, chat)
			if err != nil {
				return failConversation(nil, nil, err)
			}
		}
		transcription, audioErr := transcribeInboundAudio(ctx, runtime, thread.ID, voiceMedia)
		if audioErr != nil {
			return failConversation(nil, nil, audioErr)
		}
		if transcription.Text != "" {
			options.SpeechLanguage = transcription.Language
			turnPrompt = transcription.Text
			inbound.Payload.Text.Text = turnPrompt
			inbound.Payload.Attachments = nil
			inbound.Metadata = append(inbound.Metadata, transcriptionMetadata(voiceMedia, transcription)...)
			if err := b.relayVoiceTranscript(ctx, runtime, thread, inbound.Payload.ProviderMessageID, transcription.Text); err != nil {
				b.logf("voice transcript relay failed chat=%s thread=%s err=%v", chat.ID, thread.ID, err)
			}
		}
	}

	storedInbound, err := b.App.StoreInboundMessage(ctx, inbound)
	if err != nil {
		return failConversation(nil, nil, err)
	}
	if strings.TrimSpace(turnPrompt) == "" && len(savedPaths) > 0 {
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
	if len(savedPaths) > 0 {
		turnPrompt = injectFilesIntoPrompt(savedPaths, turnPrompt)
	}
	turnInbound.Text = prepareTurnInbound(inbound, turnPrompt)

	if runtime == nil {
		runtime, err = b.runtimeForChat(ctx, chat)
		if err != nil {
			return failConversation(storedInbound, nil, err)
		}
	}
	outbound, err := b.runStoredThreadTurn(ctx, runtime, chat, thread, turnInbound, options)
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
	if b == nil || b.App == nil {
		return fmt.Errorf("missing broker app")
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
	if !req.Context.ChatID.IsNull() {
		chat, err := b.App.Chat(ctx, req.Context.ChatID)
		if err != nil {
			return commandengine.Result{}, err
		}
		if chat != nil {
			extra, err := b.App.ResolveChatHostbridgeAllowedCommands(ctx, *chat)
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
	chat, err := b.App.Chat(ctx, chatID)
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
