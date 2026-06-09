package theater

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type createCommand struct{ Name string }
type listCommand struct{}
type subscribeCommand struct{ Name string }
type unsubscribeCommand struct{ Name string }
type postCommand struct {
	Name string
	Text string
}
type readCommand struct {
	Name  string
	Limit int
}
type statusCommand struct{ Name string }

func RegisterGobTypes(register func(any)) {
	register(createCommand{})
	register(listCommand{})
	register(subscribeCommand{})
	register(unsubscribeCommand{})
	register(postCommand{})
	register(readCommand{})
	register(statusCommand{})
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	sources := []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge}
	policy := simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent)
	return []commandengine.Definition{
		{
			Pattern:               "create <name>",
			Help:                  "Create a collaboration theater",
			Build:                 buildName[createCommand](func(name string) createCommand { return createCommand{Name: name} }),
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "list",
			Help:                  "List collaboration theaters",
			Build:                 func(req *clir.Request) (any, error) { _ = req; return listCommand{}, nil },
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "<name> subscribe",
			Help:                  "Subscribe this thread to a theater",
			Build:                 buildName[subscribeCommand](func(name string) subscribeCommand { return subscribeCommand{Name: name} }),
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "<name> unsubscribe",
			Help:                  "Unsubscribe this thread from a theater",
			Build:                 buildName[unsubscribeCommand](func(name string) unsubscribeCommand { return unsubscribeCommand{Name: name} }),
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "<name> post",
			Help:                  "Post a message to a theater; stdin is used when message args are omitted",
			Build:                 buildPost,
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "<name> read",
			Help:                  "Read recent theater messages",
			Build:                 buildRead,
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "status",
			Help:                  "Show theater subscriptions for this thread",
			Build:                 func(req *clir.Request) (any, error) { _ = req; return statusCommand{}, nil },
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "<name> status",
			Help:                  "Show this thread's status for a theater",
			Build:                 buildName[statusCommand](func(name string) statusCommand { return statusCommand{Name: name} }),
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
	if err := commandengine.RegisterPattern[createCommand](registry, "create <name>", c.handleCreate); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[listCommand](registry, "list", c.handleList); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[subscribeCommand](registry, "<name> subscribe", c.handleSubscribe); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[unsubscribeCommand](registry, "<name> unsubscribe", c.handleUnsubscribe); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[postCommand](registry, "<name> post", c.handlePost); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[readCommand](registry, "<name> read", c.handleRead); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[statusCommand](registry, "status", c.handleStatus); err != nil {
		return err
	}
	return commandengine.RegisterPattern[statusCommand](registry, "<name> status", c.handleStatus)
}

func buildName[T any](wrap func(string) T) commandengine.BuildFunc {
	return func(req *clir.Request) (any, error) {
		name := normalizeName(req.Params["name"])
		if name == "" {
			return nil, fmt.Errorf("missing theater name")
		}
		if extra := strings.TrimSpace(strings.Join(req.Extra, " ")); extra != "" {
			return nil, fmt.Errorf("unexpected theater arguments: %s", extra)
		}
		return wrap(name), nil
	}
}

func buildPost(req *clir.Request) (any, error) {
	name := normalizeName(req.Params["name"])
	if name == "" {
		return nil, fmt.Errorf("missing theater name")
	}
	return postCommand{Name: name, Text: strings.TrimSpace(strings.Join(req.Extra, " "))}, nil
}

func buildRead(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("theater <name> read", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	limit := fs.Int("limit", 20, "Maximum number of messages to read")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	if extra := strings.TrimSpace(strings.Join(fs.Args(), " ")); extra != "" {
		return nil, fmt.Errorf("unexpected theater read arguments: %s", extra)
	}
	name := normalizeName(req.Params["name"])
	if name == "" {
		return nil, fmt.Errorf("missing theater name")
	}
	return readCommand{Name: name, Limit: *limit}, nil
}

func (c *Component) handleCreate(ctx context.Context, req commandengine.Request, cmd createCommand) (commandengine.Result, error) {
	_ = req
	workspacePath := c.workspacePath(cmd.Name)
	theater, created, err := c.store.createTheater(ctx, cmd.Name, workspacePath)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := ensureWorkspace(theater.WorkspacePath, theater.Name); err != nil {
		return commandengine.Result{}, err
	}
	if !created {
		return commandengine.Result{Text: "theater already exists: " + theater.Name}, nil
	}
	return commandengine.Result{Text: "theater created: " + theater.Name}, nil
}

func (c *Component) handleList(ctx context.Context, req commandengine.Request, cmd listCommand) (commandengine.Result, error) {
	_, _ = req, cmd
	theaters, err := c.store.listTheaters(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	if len(theaters) == 0 {
		return commandengine.Result{Text: "no theaters"}, nil
	}
	lines := []string{"theaters"}
	for _, theater := range theaters {
		lines = append(lines, "- "+theater.Name)
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (c *Component) handleSubscribe(ctx context.Context, req commandengine.Request, cmd subscribeCommand) (commandengine.Result, error) {
	threadID, err := currentThreadID(req)
	if err != nil {
		return commandengine.Result{}, err
	}
	theater, err := c.store.theaterByName(ctx, cmd.Name)
	if err != nil {
		return commandengine.Result{}, err
	}
	created, err := c.store.subscribe(ctx, theater, threadID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if !created {
		return commandengine.Result{Text: "already subscribed: " + theater.Name}, nil
	}
	return commandengine.Result{Text: "subscribed: " + theater.Name}, nil
}

func (c *Component) handleUnsubscribe(ctx context.Context, req commandengine.Request, cmd unsubscribeCommand) (commandengine.Result, error) {
	threadID, err := currentThreadID(req)
	if err != nil {
		return commandengine.Result{}, err
	}
	theater, err := c.store.theaterByName(ctx, cmd.Name)
	if err != nil {
		return commandengine.Result{}, err
	}
	deleted, err := c.store.unsubscribe(ctx, theater, threadID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if !deleted {
		return commandengine.Result{Text: "not subscribed: " + theater.Name}, nil
	}
	return commandengine.Result{Text: "unsubscribed: " + theater.Name}, nil
}

func (c *Component) handlePost(ctx context.Context, req commandengine.Request, cmd postCommand) (commandengine.Result, error) {
	threadID, err := currentThreadID(req)
	if err != nil {
		return commandengine.Result{}, err
	}
	theater, err := c.store.theaterByName(ctx, cmd.Name)
	if err != nil {
		return commandengine.Result{}, err
	}
	text := strings.TrimSpace(cmd.Text)
	if text == "" {
		text = strings.TrimSpace(req.Stdin)
	}
	actor := req.Context.Actor.Resolved()
	message, err := c.store.post(ctx, theater, threadID, actor.ID, actor.Label, text)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "theater message posted: " + message.ID}, nil
}

func (c *Component) handleRead(ctx context.Context, req commandengine.Request, cmd readCommand) (commandengine.Result, error) {
	threadID := requestThreadID(req)
	theater, err := c.store.theaterByName(ctx, cmd.Name)
	if err != nil {
		return commandengine.Result{}, err
	}
	messages, err := c.store.read(ctx, theater, threadID, cmd.Limit)
	if err != nil {
		return commandengine.Result{}, err
	}
	if len(messages) == 0 {
		return commandengine.Result{Text: "no theater messages: " + theater.Name}, nil
	}
	return commandengine.Result{Text: formatMessages(theater.Name, messages)}, nil
}

func (c *Component) handleStatus(ctx context.Context, req commandengine.Request, cmd statusCommand) (commandengine.Result, error) {
	threadID, err := currentThreadID(req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if strings.TrimSpace(cmd.Name) != "" {
		theater, err := c.store.theaterByName(ctx, cmd.Name)
		if err != nil {
			return commandengine.Result{}, err
		}
		pending, err := c.store.pendingCount(ctx, theater.ID, threadID)
		if err != nil {
			return commandengine.Result{}, err
		}
		workspacePath := strings.TrimSpace(theater.WorkspacePath)
		if workspacePath == "" {
			workspacePath = c.workspacePath(theater.Name)
		}
		return commandengine.Result{Text: fmt.Sprintf("theater: %s\nworkspace: %s\nunread messages: %d", theater.Name, workspacePath, pending)}, nil
	}
	subscriptions, err := c.store.subscriptions(ctx, threadID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if len(subscriptions) == 0 {
		return commandengine.Result{Text: "no theater subscriptions"}, nil
	}
	lines := []string{"theater subscriptions"}
	for _, subscription := range subscriptions {
		theater, err := c.store.theaterByID(ctx, subscription.TheaterID)
		if err != nil {
			return commandengine.Result{}, err
		}
		pending, err := c.store.pendingCount(ctx, theater.ID, threadID)
		if err != nil {
			return commandengine.Result{}, err
		}
		lines = append(lines, fmt.Sprintf("- %s unread messages: %d", theater.Name, pending))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
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

func formatMessages(theaterName string, messages []messageRecord) string {
	lines := []string{"theater: " + theaterName}
	for _, message := range messages {
		label := strings.TrimSpace(message.ActorLabel)
		if label == "" {
			label = strings.TrimSpace(message.ActorID)
		}
		if label == "" {
			label = "unknown"
		}
		lines = append(lines, fmt.Sprintf("[%s] %s: %s", message.CreatedAt.UTC().Format("2006-01-02 15:04"), label, message.Text))
	}
	return strings.Join(lines, "\n")
}
