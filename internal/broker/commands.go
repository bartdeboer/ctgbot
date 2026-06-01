package broker

import (
	"bytes"
	"context"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	component "github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func (b *Broker) tryHandleMessageCommand(
	ctx context.Context,
	inbound component.ResolvedInbound,
	chat coremodel.Chat,
	thread coremodel.Thread,
	runtime *ChatRuntime,
) (bool, string, []coremodel.ThreadMessage, error) {
	argv, ok := commandArgv(inbound.Payload.Text.Text)
	if !ok {
		return false, "", nil, nil
	}

	result := commandengine.Result{Text: "command error: unknown command"}
	if runtime != nil && runtime.MessageCommands != nil {
		base := commandengine.Request{
			Context: commandengine.Context{
				Source:   commandengine.SourceMessage,
				Actor:    messageCommandActor(inbound.Payload),
				ChatID:   chat.ID,
				ThreadID: thread.ID,
			},
		}
		if helpReq, ok := commandengine.ParseHelpRequest(argv); ok {
			var buf bytes.Buffer
			helpArgs := []string(nil)
			helpOptions := []commandengine.HelpOption{commandengine.HelpLitDepth(1)}
			if len(helpReq.Scope) > 0 {
				helpArgs = append([]string{}, helpReq.Scope...)
				helpOptions = []commandengine.HelpOption{commandengine.HelpLitDepth(2)}
			}
			var err error
			if len(helpReq.Scope) == 0 {
				err = runtime.MessageCommands.Router.FPrintHelpIndex(ctx, &buf, base.Context.Actor)
			} else {
				err = runtime.MessageCommands.Router.FPrintHelpWithOptions(ctx, &buf, helpArgs, helpOptions, base.Context.Actor)
			}
			if err != nil {
				result = commandengine.Result{Text: "command error: " + strings.TrimSpace(err.Error())}
			} else {
				text := strings.TrimSpace(buf.String())
				if len(helpReq.Scope) == 0 && text != "" {
					text = "Available commands:\n" + text
				}
				result = commandengine.Result{Text: text}
			}
		} else if runResult, runErr := runtime.MessageCommands.Run(ctx, base, argv); runErr != nil {
			result = commandengine.Result{
				Text: "command error: " + strings.TrimSpace(runErr.Error()),
			}
		} else {
			result = runResult
		}
	}

	if passthrough := strings.TrimSpace(result.PassthroughPrompt); passthrough != "" {
		b.logf(
			"message command passthrough chat=%s thread=%s argv=%q passthrough_chars=%d",
			chat.ID,
			thread.ID,
			strings.Join(argv, " "),
			len(passthrough),
		)
		return false, passthrough, nil, nil
	}

	text := strings.TrimSpace(result.Text)
	if text == "" {
		return true, "", nil, nil
	}

	message, err := b.storeAndRelayMessage(
		ctx,
		runtime,
		chat,
		thread,
		coremodel.ThreadMessage{
			Role:       coremodel.MessageRoleSystem,
			Kind:       coremodel.MessageKindMessage,
			ActorID:    "ctgbot",
			ActorLabel: "ctgbot",
			Text:       text,
		},
		"ctgbot",
	)
	if err != nil {
		return true, "", nil, err
	}
	b.logf(
		"message command handled chat=%s thread=%s argv=%q result_chars=%d",
		chat.ID,
		thread.ID,
		strings.Join(argv, " "),
		len(text),
	)
	return true, "", []coremodel.ThreadMessage{*message}, nil
}

func messageCommandActor(payload message.InboundPayload) commandengine.Actor {
	actor := payload.ResolvedActor()
	actorID := strings.TrimSpace(actor.ID)
	if actorID == "" {
		actorID = "user"
	}
	return commandengine.Actor{
		ID:    actorID,
		Roles: append([]simplerbac.Role(nil), actor.Roles...),
	}
}

func commandArgv(text string) ([]string, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return nil, false
	}
	fields := strings.Fields(strings.TrimPrefix(text, "/"))
	if len(fields) == 0 {
		return nil, false
	}
	if at := strings.Index(fields[0], "@"); at >= 0 {
		fields[0] = fields[0][:at]
	}
	fields[0] = strings.TrimSpace(fields[0])
	if fields[0] == "" {
		return nil, false
	}
	return fields, true
}
