package theater

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	messagingdomain "github.com/bartdeboer/ctgbot/internal/messaging"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type subscribeCommand struct{ ThreadRef string }
type unsubscribeCommand struct{ ThreadRef string }
type readCommand struct {
	ThreadRef string
	Cursor    string
	Limit     int
}
type statusCommand struct{ ThreadRef string }

type listCommand struct{}

func RegisterGobTypes(register func(any)) {
	register(subscribeCommand{})
	register(unsubscribeCommand{})
	register(readCommand{})
	register(statusCommand{})
	register(listCommand{})
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	sources := []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge}
	policy := simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent)
	return []commandengine.Definition{
		{
			Pattern:               "<thread> subscribe",
			Help:                  "Subscribe this thread to updates from another thread",
			Build:                 buildThreadRef[subscribeCommand](func(ref string) subscribeCommand { return subscribeCommand{ThreadRef: ref} }),
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "<thread> unsubscribe",
			Help:                  "Unsubscribe this thread from another thread",
			Build:                 buildThreadRef[unsubscribeCommand](func(ref string) unsubscribeCommand { return unsubscribeCommand{ThreadRef: ref} }),
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "<thread> read",
			Help:                  "Read recent messages from another thread",
			Build:                 buildRead,
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "status",
			Help:                  "Show thread subscriptions for this thread",
			Build:                 func(req *clir.Request) (any, error) { _ = req; return statusCommand{}, nil },
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "list",
			Help:                  "List thread subscriptions for this thread",
			Build:                 func(req *clir.Request) (any, error) { _ = req; return listCommand{}, nil },
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "<thread> status",
			Help:                  "Show subscription status for another thread",
			Build:                 buildThreadRef[statusCommand](func(ref string) statusCommand { return statusCommand{ThreadRef: ref} }),
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
	}
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := commandengine.RegisterPattern[subscribeCommand](registry, "<thread> subscribe", c.handleSubscribe); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[unsubscribeCommand](registry, "<thread> unsubscribe", c.handleUnsubscribe); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[readCommand](registry, "<thread> read", c.handleRead); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[statusCommand](registry, "status", c.handleStatus); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[listCommand](registry, "list", c.handleList); err != nil {
		return err
	}
	return commandengine.RegisterPattern[statusCommand](registry, "<thread> status", c.handleStatus)
}

func buildThreadRef[T any](wrap func(string) T) commandengine.BuildFunc {
	return func(req *clir.Request) (any, error) {
		threadRef := strings.TrimSpace(req.Params["thread"])
		if threadRef == "" {
			return nil, fmt.Errorf("missing thread")
		}
		if extra := strings.TrimSpace(strings.Join(req.Extra, " ")); extra != "" {
			return nil, fmt.Errorf("unexpected theater arguments: %s", extra)
		}
		return wrap(threadRef), nil
	}
}

func buildRead(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("theater <thread> read", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cursor := fs.String("cursor", "", "Optional message cursor")
	after := fs.String("after", "", "Optional message cursor alias")
	limit := fs.Int("limit", 20, "Maximum number of messages to read")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	if extra := strings.TrimSpace(strings.Join(fs.Args(), " ")); extra != "" {
		return nil, fmt.Errorf("unexpected theater read arguments: %s", extra)
	}
	threadRef := strings.TrimSpace(req.Params["thread"])
	if threadRef == "" {
		return nil, fmt.Errorf("missing thread")
	}
	readCursor := strings.TrimSpace(*cursor)
	if readCursor == "" {
		readCursor = strings.TrimSpace(*after)
	}
	return readCommand{ThreadRef: threadRef, Cursor: readCursor, Limit: *limit}, nil
}

func (c *Component) handleSubscribe(ctx context.Context, req commandengine.Request, cmd subscribeCommand) (commandengine.Result, error) {
	subscriberThreadID, err := currentThreadID(req)
	if err != nil {
		return commandengine.Result{}, err
	}
	targetThread, err := c.resolveTargetThread(ctx, cmd.ThreadRef)
	if err != nil {
		return commandengine.Result{}, err
	}
	created, err := c.store.subscribe(ctx, subscriberThreadID, targetThread.ID, threadLabel(*targetThread))
	if err != nil {
		return commandengine.Result{}, err
	}
	if !created {
		return commandengine.Result{Text: "already subscribed: " + threadLabel(*targetThread)}, nil
	}
	return commandengine.Result{Text: "subscribed: " + threadLabel(*targetThread)}, nil
}

func (c *Component) handleUnsubscribe(ctx context.Context, req commandengine.Request, cmd unsubscribeCommand) (commandengine.Result, error) {
	subscriberThreadID, err := currentThreadID(req)
	if err != nil {
		return commandengine.Result{}, err
	}
	targetThread, err := c.resolveTargetThread(ctx, cmd.ThreadRef)
	if err != nil {
		return commandengine.Result{}, err
	}
	deleted, err := c.store.unsubscribe(ctx, subscriberThreadID, targetThread.ID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if !deleted {
		return commandengine.Result{Text: "not subscribed: " + threadLabel(*targetThread)}, nil
	}
	return commandengine.Result{Text: "unsubscribed: " + threadLabel(*targetThread)}, nil
}

func (c *Component) handleRead(ctx context.Context, req commandengine.Request, cmd readCommand) (commandengine.Result, error) {
	subscriberThreadID := requestThreadID(req)
	targetThread, err := c.resolveTargetThread(ctx, cmd.ThreadRef)
	if err != nil {
		return commandengine.Result{}, err
	}
	if c == nil || c.storage == nil {
		return commandengine.Result{}, fmt.Errorf("missing theater storage")
	}
	page, err := c.messages.ListMessages(ctx, req.Context.Actor, targetThread.ID, messagingdomain.ListMessagesRequest{Cursor: cmd.Cursor, Limit: cmd.Limit})
	if err != nil {
		return commandengine.Result{}, err
	}
	messages := page.Messages
	if len(messages) == 0 {
		return commandengine.Result{Text: "no theater messages: " + threadLabel(*targetThread)}, nil
	}
	if !subscriberThreadID.IsNull() {
		last := messages[len(messages)-1].CreatedAt.UTC()
		if err := c.store.markRead(ctx, subscriberThreadID, targetThread.ID, last); err != nil {
			return commandengine.Result{}, err
		}
	}
	return commandengine.Result{Text: formatMessages(threadLabel(*targetThread), messages, page.NextCursor)}, nil
}

func (c *Component) handleList(ctx context.Context, req commandengine.Request, cmd listCommand) (commandengine.Result, error) {
	_ = cmd
	return c.handleStatus(ctx, req, statusCommand{})
}

func (c *Component) handleStatus(ctx context.Context, req commandengine.Request, cmd statusCommand) (commandengine.Result, error) {
	subscriberThreadID, err := currentThreadID(req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if strings.TrimSpace(cmd.ThreadRef) != "" {
		targetThread, err := c.resolveTargetThread(ctx, cmd.ThreadRef)
		if err != nil {
			return commandengine.Result{}, err
		}
		pending, err := c.pendingCount(ctx, subscriberThreadID, targetThread.ID)
		if err != nil {
			return commandengine.Result{}, err
		}
		return commandengine.Result{Text: strings.Join([]string{
			"theater: " + threadLabel(*targetThread),
			"thread_id: " + targetThread.ID.String(),
			fmt.Sprintf("unread messages: %d", pending),
		}, "\n")}, nil
	}
	subscriptions, err := c.store.subscriptions(ctx, subscriberThreadID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if len(subscriptions) == 0 {
		return commandengine.Result{Text: "no theater subscriptions"}, nil
	}
	lines := []string{"theater subscriptions"}
	for _, subscription := range subscriptions {
		targetThreadID, err := modeluuid.Parse(subscription.TargetThreadID)
		if err != nil {
			return commandengine.Result{}, err
		}
		pending, err := c.pendingCount(ctx, subscriberThreadID, targetThreadID)
		if err != nil {
			return commandengine.Result{}, err
		}
		label := strings.TrimSpace(subscription.Label)
		if label == "" {
			label = subscription.TargetThreadID
		}
		lines = append(lines, fmt.Sprintf("- %s unread messages: %d", label, pending))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (c *Component) resolveTargetThread(ctx context.Context, ref string) (*coremodel.Thread, error) {
	if c == nil || c.messages == nil {
		return nil, fmt.Errorf("missing messaging service")
	}
	threadID, err := c.messages.ResolveThreadRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	if c.storage == nil {
		return nil, fmt.Errorf("missing theater storage")
	}
	return c.storage.Threads().GetByID(ctx, threadID)
}

func (c *Component) pendingCount(ctx context.Context, subscriberThreadID modeluuid.UUID, targetThreadID modeluuid.UUID) (int64, error) {
	if c == nil || c.storage == nil {
		return 0, fmt.Errorf("missing theater storage")
	}
	subscription, ok, err := c.store.subscription(ctx, subscriberThreadID, targetThreadID)
	if err != nil || !ok {
		return 0, err
	}
	return c.storage.Messages().CountByThreadIDSince(ctx, targetThreadID, subscription.LastReadAt)
}

func requestThreadID(req commandengine.Request) modeluuid.UUID {
	if !req.Context.ThreadID.IsNull() {
		return req.Context.ThreadID
	}
	return req.Context.SandboxID
}

func currentThreadID(req commandengine.Request) (modeluuid.UUID, error) {
	threadID := requestThreadID(req)
	if threadID.IsNull() {
		return modeluuid.Nil, fmt.Errorf("current thread is unavailable")
	}
	return threadID, nil
}

func threadLabel(thread coremodel.Thread) string {
	label := strings.TrimSpace(thread.Label)
	if label == "" {
		label = thread.ID.String()
	}
	return label
}

func formatMessages(theaterName string, messages []coremodel.ThreadMessage, nextCursor string) string {
	lines := []string{"theater: " + theaterName}
	for _, msg := range messages {
		label := strings.TrimSpace(msg.ActorLabel)
		if label == "" {
			label = strings.TrimSpace(msg.ActorID)
		}
		if label == "" {
			label = "unknown"
		}
		lines = append(lines, fmt.Sprintf("[%s] %s: %s", msg.CreatedAt.UTC().Format("2006-01-02 15:04"), label, msg.Text))
	}
	if strings.TrimSpace(nextCursor) != "" {
		lines = append(lines, "", "next cursor: "+strings.TrimSpace(nextCursor))
	}
	return strings.Join(lines, "\n")
}
