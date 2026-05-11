package messagingcomponent

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	messagingdomain "github.com/bartdeboer/ctgbot/internal/messaging"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

const Type = "messaging"

type Component struct {
	Service *messagingdomain.Service
	Inbound component.ResolvedInboundQueuer
}

var _ component.Component = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)

type listCommand struct {
	Limit int
	Query string
}

type currentStatusCommand struct{}

type statusCommand struct {
	ThreadRef string
}

type labelSetCommand struct {
	ThreadRef string
	Label     string
}

type messageListCommand struct {
	ThreadRef string
	Cursor    string
	Limit     int
}

type messageSendCommand struct {
	ThreadRef string
	Text      string
}

func RegisterGobTypes(register func(any)) {
	register(listCommand{})
	register(currentStatusCommand{})
	register(statusCommand{})
	register(labelSetCommand{})
	register(messageListCommand{})
	register(messageSendCommand{})
}

func New(service *messagingdomain.Service, inbound component.ResolvedInboundQueuer) *Component {
	return &Component{Service: service, Inbound: inbound}
}

func (c *Component) Type() string { return Type }

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		{
			Pattern: "status",
			Help:    "Show current thread status",
			Build: func(req *clir.Request) (any, error) {
				if extra := strings.TrimSpace(strings.Join(req.Extra, " ")); extra != "" {
					return nil, fmt.Errorf("unexpected status arguments: %s", extra)
				}
				return currentStatusCommand{}, nil
			},
			Sources:               []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:                simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser),
			InstructionVisibility: commandengine.InstructionEssential,
			Aliases: []commandengine.Route{
				{Pattern: "thread status", Absolute: true},
				{Pattern: "thread current status", Absolute: true},
			},
		},
		{
			Pattern: "thread <thread> status",
			Help:    "Show referenced thread status",
			Build:   buildStatusCommand,
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
		},
		{
			Pattern: "thread <thread> label set",
			Help:    "Set thread label",
			Build:   buildLabelSetCommand,
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
			Aliases: []commandengine.Route{
				{Pattern: "thread label set", Absolute: true},
			},
		},
		{
			Pattern:               "thread list",
			Help:                  "List recent active threads",
			Build:                 buildListCommand,
			Sources:               []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:                simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "thread <thread> message list",
			Help:    "List messages for a thread",
			Build:   buildMessageListCommand,
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
		},
		{
			Pattern:               "thread <thread> message send",
			Help:                  "Send a message into another thread",
			Build:                 buildMessageSendCommand,
			Sources:               []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:                simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
			InstructionVisibility: commandengine.InstructionImportant,
		},
	}
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := commandengine.Register[listCommand](registry, c.handleList); err != nil {
		return err
	}
	if err := commandengine.Register[currentStatusCommand](registry, c.handleCurrentStatus); err != nil {
		return err
	}
	if err := commandengine.Register[statusCommand](registry, c.handleStatus); err != nil {
		return err
	}
	if err := commandengine.Register[labelSetCommand](registry, c.handleLabelSet); err != nil {
		return err
	}
	if err := commandengine.Register[messageListCommand](registry, c.handleMessageList); err != nil {
		return err
	}
	return commandengine.Register[messageSendCommand](registry, c.handleMessageSend)
}

func (c *Component) handleList(ctx context.Context, req commandengine.Request, cmd listCommand) (commandengine.Result, error) {
	if c == nil || c.Service == nil {
		return commandengine.Result{}, fmt.Errorf("missing messaging service")
	}
	threads, err := c.Service.ListThreads(ctx, req.Context.Actor, messagingdomain.ListThreadsRequest{
		Limit: cmd.Limit,
		Query: cmd.Query,
	})
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{
		Text: formatThreadList(threads, requestThreadID(req)),
	}, nil
}

func (c *Component) handleCurrentStatus(ctx context.Context, req commandengine.Request, cmd currentStatusCommand) (commandengine.Result, error) {
	_ = cmd
	return c.threadStatus(ctx, req, "current")
}

func (c *Component) handleStatus(ctx context.Context, req commandengine.Request, cmd statusCommand) (commandengine.Result, error) {
	return c.threadStatus(ctx, req, cmd.ThreadRef)
}

func (c *Component) threadStatus(ctx context.Context, req commandengine.Request, threadRef string) (commandengine.Result, error) {
	if c == nil || c.Service == nil {
		return commandengine.Result{}, fmt.Errorf("missing messaging service")
	}
	threadID, err := c.resolveThreadID(ctx, req, threadRef)
	if err != nil {
		return commandengine.Result{}, err
	}
	status, err := c.Service.ThreadStatus(ctx, req.Context.Actor, threadID)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: formatThreadStatus(status)}, nil
}

func (c *Component) handleLabelSet(ctx context.Context, req commandengine.Request, cmd labelSetCommand) (commandengine.Result, error) {
	if c == nil || c.Service == nil {
		return commandengine.Result{}, fmt.Errorf("missing messaging service")
	}
	threadID, err := c.resolveThreadID(ctx, req, cmd.ThreadRef)
	if err != nil {
		return commandengine.Result{}, err
	}
	status, err := c.Service.SetThreadLabel(ctx, req.Context.Actor, threadID, cmd.Label)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "thread label set: " + strings.TrimSpace(status.Label)}, nil
}

func (c *Component) handleMessageList(ctx context.Context, req commandengine.Request, cmd messageListCommand) (commandengine.Result, error) {
	if c == nil || c.Service == nil {
		return commandengine.Result{}, fmt.Errorf("missing messaging service")
	}
	threadID, err := c.resolveThreadID(ctx, req, cmd.ThreadRef)
	if err != nil {
		return commandengine.Result{}, err
	}
	page, err := c.Service.ListMessages(ctx, req.Context.Actor, threadID, messagingdomain.ListMessagesRequest{
		Cursor: cmd.Cursor,
		Limit:  cmd.Limit,
	})
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{
		Text: formatMessagePage(threadID, page),
	}, nil
}

func (c *Component) handleMessageSend(ctx context.Context, req commandengine.Request, cmd messageSendCommand) (commandengine.Result, error) {
	if c == nil || c.Service == nil {
		return commandengine.Result{}, fmt.Errorf("missing messaging service")
	}
	if c.Inbound == nil {
		return commandengine.Result{}, fmt.Errorf("missing resolved inbound handler")
	}
	threadID, err := c.resolveThreadID(ctx, req, cmd.ThreadRef)
	if err != nil {
		return commandengine.Result{}, err
	}
	targetChat, targetThread, err := c.Service.ThreadTarget(ctx, threadID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if !targetChat.Enabled {
		return commandengine.Result{}, fmt.Errorf("target chat is disabled: %s", targetChat.ID)
	}
	actor := req.Context.Actor.Resolved()
	sourceThreadID := requestThreadID(req)
	if !sourceThreadID.IsNull() {
		if sourceThreadID == threadID {
			return commandengine.Result{}, fmt.Errorf("cannot send thread message to the current thread")
		}
		threadActor, err := c.Service.ActorForThread(ctx, sourceThreadID)
		if err != nil {
			return commandengine.Result{}, err
		}
		threadActor.Roles = append([]simplerbac.Role(nil), actor.Roles...)
		actor = threadActor
	}
	inbound := component.ResolvedInbound{
		Chat:   *targetChat,
		Thread: *targetThread,
		Payload: message.InboundPayload{
			ProviderType: "thread",
			Text:         message.TextMessage{Text: strings.TrimSpace(cmd.Text)},
			Actor:        actor,
		},
		PromptContext: &component.InboundPromptContext{
			Kind:      "Internal thread message",
			FromLabel: actor.Label,
			FromID:    actor.ID,
		},
	}
	if !sourceThreadID.IsNull() {
		inbound.Metadata = append(inbound.Metadata, "source_thread_id="+sourceThreadID.String())
		inbound.PromptContext.ReplyHint = "hostbridge thread " + sourceThreadID.String() + " message send <message>"
	}
	if err := c.Inbound.QueueResolvedInbound(ctx, inbound); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "message queued"}, nil
}

func (c *Component) resolveThreadID(ctx context.Context, req commandengine.Request, ref string) (modeluuid.UUID, error) {
	ref = strings.TrimSpace(ref)
	if strings.EqualFold(ref, "current") {
		threadID := requestThreadID(req)
		if threadID.IsNull() {
			return modeluuid.Nil, fmt.Errorf("current thread is unavailable")
		}
		return threadID, nil
	}
	return c.Service.ResolveThreadRef(ctx, ref)
}

func requestThreadID(req commandengine.Request) modeluuid.UUID {
	if !req.Context.ThreadID.IsNull() {
		return req.Context.ThreadID
	}
	return req.Context.SandboxID
}

func buildListCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("thread list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	limit := fs.Int("limit", 20, "Maximum number of threads")
	query := fs.String("query", "", "Optional case-insensitive search filter")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	return listCommand{
		Limit: *limit,
		Query: strings.TrimSpace(*query),
	}, nil
}

func buildMessageListCommand(req *clir.Request) (any, error) {
	threadRef := strings.TrimSpace(req.Params["thread"])
	if threadRef == "" {
		return nil, fmt.Errorf("missing thread")
	}
	fs := flag.NewFlagSet("thread message list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cursor := fs.String("cursor", "", "Optional message cursor")
	limit := fs.Int("limit", 20, "Maximum number of messages")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	return messageListCommand{
		ThreadRef: threadRef,
		Cursor:    strings.TrimSpace(*cursor),
		Limit:     *limit,
	}, nil
}

func buildMessageSendCommand(req *clir.Request) (any, error) {
	threadRef := strings.TrimSpace(req.Params["thread"])
	if threadRef == "" {
		return nil, fmt.Errorf("missing thread")
	}
	text := strings.TrimSpace(strings.Join(req.Extra, " "))
	if text == "" {
		return nil, fmt.Errorf("missing message")
	}
	return messageSendCommand{
		ThreadRef: threadRef,
		Text:      text,
	}, nil
}

func buildStatusCommand(req *clir.Request) (any, error) {
	threadRef := strings.TrimSpace(req.Params["thread"])
	if threadRef == "" {
		threadRef = "current"
	}
	if extra := strings.TrimSpace(strings.Join(req.Extra, " ")); extra != "" {
		return nil, fmt.Errorf("unexpected status arguments: %s", extra)
	}
	return statusCommand{ThreadRef: threadRef}, nil
}

func buildLabelSetCommand(req *clir.Request) (any, error) {
	threadRef := strings.TrimSpace(req.Params["thread"])
	if threadRef == "" {
		threadRef = "current"
	}
	label := strings.TrimSpace(strings.Join(req.Extra, " "))
	if label == "" {
		return nil, fmt.Errorf("missing thread label")
	}
	return labelSetCommand{
		ThreadRef: threadRef,
		Label:     label,
	}, nil
}

func formatThreadList(threads []messagingdomain.ThreadSummary, currentThreadID modeluuid.UUID) string {
	if len(threads) == 0 {
		return "no recent threads"
	}
	threads = append([]messagingdomain.ThreadSummary(nil), threads...)
	sort.SliceStable(threads, func(i, j int) bool {
		if threads[i].LastMessageAt.Equal(threads[j].LastMessageAt) {
			return threads[i].ID.String() < threads[j].ID.String()
		}
		return threads[i].LastMessageAt.After(threads[j].LastMessageAt)
	})
	lines := []string{"Recent threads:"}
	for _, thread := range threads {
		threadRef := strings.TrimSpace(thread.ShortID)
		if threadRef == "" {
			threadRef = thread.ID.String()
		}
		label := strings.TrimSpace(thread.ChatLabel)
		if thread.ThreadLabel != "" {
			if label != "" {
				label += " / "
			}
			label += thread.ThreadLabel
		}
		if label == "" {
			label = thread.ChatID.String()
		}
		marker := ""
		if thread.ID == currentThreadID {
			marker = " (current)"
		}
		last := strings.TrimSpace(thread.LastMessageText)
		if len(last) > 80 {
			last = last[:80]
		}
		if last == "" {
			lines = append(lines, fmt.Sprintf("%s%s - %s", threadRef, marker, label))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s%s - %s - %s", threadRef, marker, label, last))
	}
	return strings.Join(lines, "\n")
}

func formatThreadStatus(status messagingdomain.ThreadStatus) string {
	lines := []string{
		"thread status",
		"chat_id: " + status.ChatID.String(),
		"chat_short_id: " + strings.TrimSpace(status.ChatShortID),
		"chat_label: " + strings.TrimSpace(status.ChatLabel),
		"chat_enabled: " + fmt.Sprintf("%t", status.ChatEnabled),
		"thread_id: " + status.ID.String(),
		"thread_short_id: " + strings.TrimSpace(status.ShortID),
		"thread_label: " + strings.TrimSpace(status.Label),
	}
	if len(status.Components) == 0 {
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "components:")
	for _, component := range status.Components {
		line := "- " + strings.TrimSpace(component.Ref)
		if role := strings.TrimSpace(component.Role); role != "" {
			line += " " + role
		}
		if externalChatID := strings.TrimSpace(component.ExternalChatID); externalChatID != "" {
			line += " external_chat_id=" + externalChatID
		}
		if externalThreadID := strings.TrimSpace(component.ExternalThreadID); externalThreadID != "" {
			line += " external_thread_id=" + externalThreadID
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func formatMessagePage(threadID modeluuid.UUID, page messagingdomain.MessagePage) string {
	if len(page.Messages) == 0 {
		return "no messages for thread " + threadID.String()
	}
	lines := []string{"Messages for thread " + threadID.String() + ":"}
	for _, msg := range page.Messages {
		lines = append(lines, fmt.Sprintf(
			"%s %s %s %s",
			msg.CreatedAt.Format(time.RFC3339),
			shortID(msg.ID),
			formatMessageActor(msg),
			strings.TrimSpace(msg.Text),
		))
	}
	if strings.TrimSpace(page.NextCursor) != "" {
		lines = append(lines, "")
		lines = append(lines, "next cursor: "+page.NextCursor)
	}
	return strings.Join(lines, "\n")
}

func formatMessageActor(msg coremodel.ThreadMessage) string {
	label := strings.TrimSpace(msg.ActorLabel)
	if label == "" {
		label = strings.TrimSpace(msg.ActorID)
	}
	if label == "" {
		label = "unknown"
	}
	return "[" + label + "]"
}

func shortID(id modeluuid.UUID) string {
	value := id.String()
	if len(value) <= 8 {
		return value
	}
	return value[:8]
}
