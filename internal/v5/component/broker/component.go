package broker

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
)

const Type = "broker"

type Actions interface {
	SendPayload(ctx context.Context, threadID modeluuid.UUID, payload messenger.OutboundPayload) error
}

type Component struct {
	Actions Actions
}

var _ component.Component = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)

func New(actions Actions) *Component {
	return &Component{Actions: actions}
}

func (c *Component) Type() string {
	return Type
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	definitions := schemacommands.HostbridgeCommands()
	var out []commandengine.Definition
	for _, definition := range definitions {
		switch definition.ID {
		case "hostbridge.sendfile", "hostbridge.sendstdin":
			out = append(out, definition)
		}
	}
	return out
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
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
