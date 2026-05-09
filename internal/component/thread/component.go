package thread

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
	"github.com/bartdeboer/ctgbot/internal/messaging"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

const Type = "thread"

type Actions interface {
	messaging.LocalActions
}

type Component struct {
	Actions Actions
}

var _ component.Component = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)

type listCommand struct {
	Limit int
	Query string
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
	register(messageListCommand{})
	register(messageSendCommand{})
}

func New(actions Actions) *Component {
	return &Component{Actions: actions}
}

func (c *Component) Type() string { return Type }

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		{
			Pattern: "thread list",
			Help:    "List recent active threads",
			Build:   buildListCommand,
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
		},
		{
			Pattern: "thread <thread> message list",
			Help:    "List messages for a thread",
			Build:   buildMessageListCommand,
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
		},
		{
			Pattern: "thread <thread> message send",
			Help:    "Send a message into another thread",
			Build:   buildMessageSendCommand,
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
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
	if err := commandengine.Register[messageListCommand](registry, c.handleMessageList); err != nil {
		return err
	}
	return commandengine.Register[messageSendCommand](registry, c.handleMessageSend)
}

func (c *Component) handleList(ctx context.Context, req commandengine.Request, cmd listCommand) (commandengine.Result, error) {
	if c == nil || c.Actions == nil {
		return commandengine.Result{}, fmt.Errorf("missing thread actions")
	}
	threads, err := c.Actions.ListThreads(ctx, req.Context.Actor, messaging.ListThreadsRequest{
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

func (c *Component) handleMessageList(ctx context.Context, req commandengine.Request, cmd messageListCommand) (commandengine.Result, error) {
	if c == nil || c.Actions == nil {
		return commandengine.Result{}, fmt.Errorf("missing thread actions")
	}
	threadID, err := c.resolveThreadID(ctx, req, cmd.ThreadRef)
	if err != nil {
		return commandengine.Result{}, err
	}
	page, err := c.Actions.ListMessages(ctx, req.Context.Actor, threadID, messaging.ListMessagesRequest{
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
	if c == nil || c.Actions == nil {
		return commandengine.Result{}, fmt.Errorf("missing thread actions")
	}
	threadID, err := c.resolveThreadID(ctx, req, cmd.ThreadRef)
	if err != nil {
		return commandengine.Result{}, err
	}
	actor := req.Context.Actor.Resolved()
	sourceThreadID := requestThreadID(req)
	if !sourceThreadID.IsNull() {
		threadActor, err := c.Actions.ActorForThread(ctx, sourceThreadID)
		if err != nil {
			return commandengine.Result{}, err
		}
		threadActor.Roles = append([]simplerbac.Role(nil), actor.Roles...)
		actor = threadActor
	}
	result, err := c.Actions.SendMessage(ctx, actor, threadID, messaging.SendMessageRequest{
		Text:           cmd.Text,
		SourceThreadID: sourceThreadID,
	})
	if err != nil {
		return commandengine.Result{}, err
	}
	if result == nil {
		return commandengine.Result{Text: "message sent"}, nil
	}
	return commandengine.Result{
		Text: "message sent\nmessage_id: " + result.Message.ID.String(),
	}, nil
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
	return c.Actions.ResolveThreadRef(ctx, ref)
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

func formatThreadList(threads []messaging.ThreadSummary, currentThreadID modeluuid.UUID) string {
	if len(threads) == 0 {
		return "no recent threads"
	}
	threads = append([]messaging.ThreadSummary(nil), threads...)
	sort.SliceStable(threads, func(i, j int) bool {
		if threads[i].LastMessageAt.Equal(threads[j].LastMessageAt) {
			return threads[i].ID.String() < threads[j].ID.String()
		}
		return threads[i].LastMessageAt.After(threads[j].LastMessageAt)
	})
	lines := []string{"Recent threads:"}
	for _, thread := range threads {
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
			lines = append(lines, fmt.Sprintf("%s%s - %s", thread.ID.String(), marker, label))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s%s - %s - %s", thread.ID.String(), marker, label, last))
	}
	return strings.Join(lines, "\n")
}

func formatMessagePage(threadID modeluuid.UUID, page messaging.MessagePage) string {
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
