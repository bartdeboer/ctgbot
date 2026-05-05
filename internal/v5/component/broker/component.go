package broker

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/go-clir"
)

const Type = "broker"

type Actions interface {
	SendPayload(ctx context.Context, threadID modeluuid.UUID, payload messenger.OutboundPayload) error
	RunHostbridgeCommand(ctx context.Context, req commandengine.Request, cmd schemacommands.RunCommand) (commandengine.Result, error)
	MessageHelp(ctx context.Context, chatID modeluuid.UUID) (string, error)
}

type Component struct {
	Actions Actions
}

var _ component.Component = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)

type helpCommand struct{}

func New(actions Actions) *Component {
	return &Component{Actions: actions}
}

func (c *Component) Type() string {
	return Type
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	definitions := schemacommands.HostbridgeCommands()
	out := []commandengine.Definition{
		{
			ID:      "broker.help",
			Sources: []commandengine.Source{commandengine.SourceMessage},
			Policy:  simplerbac.Public(),
			Routes: []commandengine.Route{{
				Pattern: "help",
				Help:    "Show available commands",
				Build: func(req *clir.Request) (any, error) {
					_ = req
					return helpCommand{}, nil
				},
			}},
		},
	}
	for _, definition := range definitions {
		switch definition.ID {
		case "hostbridge.run", "hostbridge.sendfile", "hostbridge.sendstdin":
			out = append(out, definition)
		}
	}
	return out
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
			text, err := c.Actions.MessageHelp(ctx, req.Context.ChatID)
			if err != nil {
				return commandengine.Result{}, err
			}
			return commandengine.Result{Text: text}, nil
		},
	); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.RunCommand](
		registry,
		func(ctx context.Context, req commandengine.Request, cmd schemacommands.RunCommand) (commandengine.Result, error) {
			if c == nil || c.Actions == nil {
				return commandengine.Result{}, fmt.Errorf("missing broker actions")
			}
			return c.Actions.RunHostbridgeCommand(ctx, req, cmd)
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

func (c *Component) sendMedia(
	ctx context.Context,
	req commandengine.Request,
	cmd schemacommands.SendMedia,
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
	return c.Actions.SendPayload(
		ctx,
		threadID,
		messenger.OutboundPayload{
			Text: messenger.TextMessage{
				Text: cmd.Caption,
			},
			Attachments: []messenger.Media{{
				Filename:    cmd.Filename,
				ContentType: cmd.ContentType,
				Syntax:      cmd.Syntax,
				Content:     append([]byte(nil), cmd.Content...),
			}},
		},
	)
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
		for _, route := range definition.Routes {
			pattern := "/" + commandengine.NormalizePattern(route.Pattern)
			text := pattern
			help := strings.TrimSpace(route.Help)
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

func DefaultAllowedCommands(extra map[string]hostbridgeserver.AllowedCommand) map[string]hostbridgeserver.AllowedCommand {
	return hostbridgeserver.MergeNamedAllowedCommands(extra)
}
