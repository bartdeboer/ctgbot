package broker

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

const Type = "broker"

type Actions interface {
	SendPayload(ctx context.Context, threadID modeluuid.UUID, payload message.OutboundPayload) error
	RunHostbridgeAlias(ctx context.Context, req commandengine.Request, cmd schemacommands.RunCommand) (commandengine.Result, error)
	MessageHelp(ctx context.Context, chatID modeluuid.UUID, actor commandengine.Actor) (string, error)
	RefreshThreadRuntime(ctx context.Context, threadID modeluuid.UUID) (string, error)
	StartThreadRuntime(ctx context.Context, threadID modeluuid.UUID) (string, error)
	StopThreadRuntime(ctx context.Context, threadID modeluuid.UUID) (string, error)
	RefreshAllThreadRuntimes(ctx context.Context) (string, error)
	StartAllKeepRunningThreadRuntimes(ctx context.Context) (string, error)
	StopAllKeepRunningThreadRuntimes(ctx context.Context) (string, error)
	DroppedList(ctx context.Context, limit int) (string, error)
	DroppedView(ctx context.Context, ref string) (string, error)
	DroppedAllow(ctx context.Context, ref string) (string, error)
}

type Component struct {
	Actions Actions
}

var _ component.Component = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)
var _ component.CommandDescriptionSurface = (*Component)(nil)

type helpCommand struct{}
type containerRefreshCommand struct{ All bool }
type containerStartCommand struct{ All bool }
type containerStopCommand struct{ All bool }

type droppedListCommand struct {
	Limit int
}

type droppedViewCommand struct {
	DropRef string
}

type droppedAllowCommand struct {
	DropRef string
}

func New(actions Actions) *Component {
	return &Component{Actions: actions}
}

func containerCommand(pattern string, help string, command any, rootOnly bool) commandengine.Definition {
	definition := commandengine.Definition{
		Pattern:               pattern,
		Help:                  help,
		Build:                 func(_ *clir.Request) (any, error) { return command, nil },
		Sources:               []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
		Policy:                simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser),
		InstructionVisibility: commandengine.InstructionImportant,
	}
	if rootOnly {
		definition.Policy = simplerbac.Any(simplerbac.RoleRoot)
	}
	if pattern == "container refresh" {
		definition.Aliases = []commandengine.Route{{Pattern: "refresh", Hidden: true}}
	}
	return definition
}

func RegisterGobTypes(register func(any)) {
	register(containerRefreshCommand{})
	register(containerStartCommand{})
	register(containerStopCommand{})
	register(droppedListCommand{})
	register(droppedViewCommand{})
	register(droppedAllowCommand{})
}

func (c *Component) Type() string {
	return Type
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	definitions := schemacommands.HostbridgeCommands()
	out := []commandengine.Definition{
		containerCommand("container refresh", "Refresh the current thread runtime container", containerRefreshCommand{}, false),
		containerCommand("container start", "Start the current thread runtime container and mark it keep-running", containerStartCommand{}, false),
		containerCommand("container stop", "Stop the current thread runtime container and clear keep-running", containerStopCommand{}, false),
		containerCommand("container refresh all", "Refresh all ctgbot-managed thread runtime containers", containerRefreshCommand{All: true}, true),
		containerCommand("container start all", "Start all keep-running thread runtime containers", containerStartCommand{All: true}, true),
		containerCommand("container stop all", "Stop all keep-running thread runtime containers without changing keep-running", containerStopCommand{All: true}, true),
		{
			Pattern: "help",
			Help:    "Show available commands",
			Build: func(req *clir.Request) (any, error) {
				_ = req
				return helpCommand{}, nil
			},
			Sources: []commandengine.Source{commandengine.SourceMessage},
			Policy:  simplerbac.Public(),
		},
		{
			Pattern: "dropped list",
			Help:    "List dropped inbound messages",
			Build: func(req *clir.Request) (any, error) {
				_ = req
				return droppedListCommand{Limit: 20}, nil
			},
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
		},
		{
			Pattern: "dropped view <dropID>",
			Help:    "View a dropped inbound message",
			Build: func(req *clir.Request) (any, error) {
				return droppedViewCommand{DropRef: strings.TrimSpace(req.Params["dropID"])}, nil
			},
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
		},
		{
			Pattern: "dropped allow <dropID>",
			Help:    "Replay a dropped inbound message, bypassing event filters",
			Build: func(req *clir.Request) (any, error) {
				return droppedAllowCommand{DropRef: strings.TrimSpace(req.Params["dropID"])}, nil
			},
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
			Aliases: []commandengine.Route{
				{Pattern: "dropped <dropID> allow", Hidden: true},
			},
		},
	}
	for _, definition := range definitions {
		switch definition.CanonicalPattern() {
		case "run <command>", "send <text>", "send stdin", "sendfile <path>", "sendfile stdin",
			"turn info",
			"turn config list", "turn config get <key>", "turn config set <key> <value>", "turn config unset <key>":
			out = append(out, definition)
		}
	}
	return out
}

func (c *Component) CommandDescriptions() []commandengine.Description {
	return schemacommands.HostbridgeCommandDescriptions()
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := commandengine.Register[helpCommand](
		registry,
		func(ctx context.Context, req commandengine.Request, cmd helpCommand) (commandengine.Result, error) {
			_, _ = cmd, c
			if c == nil || c.Actions == nil {
				return commandengine.Result{}, fmt.Errorf("missing broker actions")
			}
			text, err := c.Actions.MessageHelp(ctx, req.Context.ChatID, req.Context.Actor)
			if err != nil {
				return commandengine.Result{}, err
			}
			return commandengine.Result{Text: text}, nil
		},
	); err != nil {
		return err
	}
	if err := commandengine.Register[containerRefreshCommand](registry, c.handleContainerRefresh); err != nil {
		return err
	}
	if err := commandengine.Register[containerStartCommand](registry, c.handleContainerStart); err != nil {
		return err
	}
	if err := commandengine.Register[containerStopCommand](registry, c.handleContainerStop); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.RunCommand](
		registry,
		func(ctx context.Context, req commandengine.Request, cmd schemacommands.RunCommand) (commandengine.Result, error) {
			if c == nil || c.Actions == nil {
				return commandengine.Result{}, fmt.Errorf("missing broker actions")
			}
			return c.Actions.RunHostbridgeAlias(ctx, req, cmd)
		},
	); err != nil {
		return err
	}
	if err := commandengine.Register[droppedListCommand](
		registry,
		func(ctx context.Context, req commandengine.Request, cmd droppedListCommand) (commandengine.Result, error) {
			_, _ = req, cmd
			if c == nil || c.Actions == nil {
				return commandengine.Result{}, fmt.Errorf("missing broker actions")
			}
			text, err := c.Actions.DroppedList(ctx, cmd.Limit)
			if err != nil {
				return commandengine.Result{}, err
			}
			return commandengine.Result{Text: text}, nil
		},
	); err != nil {
		return err
	}
	if err := commandengine.Register[droppedViewCommand](
		registry,
		func(ctx context.Context, req commandengine.Request, cmd droppedViewCommand) (commandengine.Result, error) {
			_ = req
			if c == nil || c.Actions == nil {
				return commandengine.Result{}, fmt.Errorf("missing broker actions")
			}
			text, err := c.Actions.DroppedView(ctx, cmd.DropRef)
			if err != nil {
				return commandengine.Result{}, err
			}
			return commandengine.Result{Text: text}, nil
		},
	); err != nil {
		return err
	}
	if err := commandengine.Register[droppedAllowCommand](
		registry,
		func(ctx context.Context, req commandengine.Request, cmd droppedAllowCommand) (commandengine.Result, error) {
			_ = req
			if c == nil || c.Actions == nil {
				return commandengine.Result{}, fmt.Errorf("missing broker actions")
			}
			text, err := c.Actions.DroppedAllow(ctx, cmd.DropRef)
			if err != nil {
				return commandengine.Result{}, err
			}
			return commandengine.Result{Text: text}, nil
		},
	); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.SendPayload](
		registry,
		func(ctx context.Context, req commandengine.Request, cmd schemacommands.SendPayload) (commandengine.Result, error) {
			if err := c.sendPayload(ctx, req, cmd.Payload); err != nil {
				return commandengine.Result{}, err
			}
			return commandengine.Result{}, nil
		},
	); err != nil {
		return err
	}
	return commandengine.Register[schemacommands.SendMedia](
		registry,
		func(ctx context.Context, req commandengine.Request, cmd schemacommands.SendMedia) (commandengine.Result, error) {
			if err := c.sendMedia(ctx, req, cmd); err != nil {
				return commandengine.Result{}, err
			}
			return commandengine.Result{}, nil
		},
	)
}

func (c *Component) handleContainerRefresh(ctx context.Context, req commandengine.Request, cmd containerRefreshCommand) (commandengine.Result, error) {
	if c == nil || c.Actions == nil {
		return commandengine.Result{}, fmt.Errorf("missing broker actions")
	}
	var (
		text string
		err  error
	)
	if cmd.All {
		text, err = c.Actions.RefreshAllThreadRuntimes(ctx)
	} else {
		text, err = c.Actions.RefreshThreadRuntime(ctx, c.currentThreadID(req))
	}
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: text}, nil
}

func (c *Component) handleContainerStart(ctx context.Context, req commandengine.Request, cmd containerStartCommand) (commandengine.Result, error) {
	if c == nil || c.Actions == nil {
		return commandengine.Result{}, fmt.Errorf("missing broker actions")
	}
	var (
		text string
		err  error
	)
	if cmd.All {
		text, err = c.Actions.StartAllKeepRunningThreadRuntimes(ctx)
	} else {
		text, err = c.Actions.StartThreadRuntime(ctx, c.currentThreadID(req))
	}
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: text}, nil
}

func (c *Component) handleContainerStop(ctx context.Context, req commandengine.Request, cmd containerStopCommand) (commandengine.Result, error) {
	if c == nil || c.Actions == nil {
		return commandengine.Result{}, fmt.Errorf("missing broker actions")
	}
	var (
		text string
		err  error
	)
	if cmd.All {
		text, err = c.Actions.StopAllKeepRunningThreadRuntimes(ctx)
	} else {
		text, err = c.Actions.StopThreadRuntime(ctx, c.currentThreadID(req))
	}
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: text}, nil
}

func (c *Component) currentThreadID(req commandengine.Request) modeluuid.UUID {
	threadID := req.Context.ThreadID
	if threadID.IsNull() {
		threadID = req.Context.SandboxID
	}
	return threadID
}

func (c *Component) sendPayload(
	ctx context.Context,
	req commandengine.Request,
	payload message.OutboundPayload,
) error {
	if c == nil || c.Actions == nil {
		return fmt.Errorf("missing broker actions")
	}
	threadID := req.Context.ThreadID
	if threadID.IsNull() {
		threadID = req.Context.SandboxID
	}
	if threadID.IsNull() {
		return fmt.Errorf("missing thread id")
	}
	if strings.TrimSpace(payload.Text.Text) == "" && len(payload.Attachments) == 0 {
		payload.Text.Text = strings.TrimSpace(req.Stdin)
	}
	if payload.IsZero() {
		return fmt.Errorf("message requires text, stdin, or --attach")
	}
	return c.Actions.SendPayload(ctx, threadID, payload)
}

func (c *Component) sendMedia(
	ctx context.Context,
	req commandengine.Request,
	cmd schemacommands.SendMedia,
) error {
	return c.sendPayload(ctx, req, message.OutboundPayload{
		Text: message.TextMessage{
			Text: cmd.Caption,
		},
		Attachments: []message.Media{{
			Filename:          cmd.Filename,
			ContentType:       cmd.ContentType,
			Syntax:            cmd.Syntax,
			Content:           append([]byte(nil), cmd.Content...),
			Width:             cmd.Width,
			Height:            cmd.Height,
			DurationSeconds:   cmd.DurationSeconds,
			SupportsStreaming: cmd.SupportsStreaming,
			Thumbnail:         cmd.Thumbnail,
		}},
	})
}

func FormatHelp(definitions []commandengine.Definition) string {
	if len(definitions) == 0 {
		return "Commands:\n/help"
	}
	type line struct {
		pattern string
		text    string
	}
	lines := make([]line, 0)
	for _, definition := range definitions {
		for _, route := range definition.Routes() {
			if route.Hidden {
				continue
			}
			pattern := "/" + commandengine.NormalizePattern(route.Pattern)
			text := pattern
			help := strings.TrimSpace(definition.Help)
			if help != "" {
				text += " - " + help
			}
			lines = append(lines, line{pattern: pattern, text: text})
		}
	}
	sort.Slice(lines, func(i, j int) bool {
		return lines[i].pattern < lines[j].pattern
	})
	out := make([]string, 0, len(lines))
	seen := map[string]struct{}{}
	for _, line := range lines {
		if _, ok := seen[line.pattern]; ok {
			continue
		}
		seen[line.pattern] = struct{}{}
		out = append(out, line.text)
	}
	if len(out) == 0 {
		return "Commands:\n/help"
	}
	return "Commands:\n" + strings.Join(out, "\n")
}
