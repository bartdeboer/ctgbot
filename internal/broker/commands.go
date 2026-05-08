package broker

import (
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
	event component.InboundEvent,
	chat coremodel.Chat,
	thread coremodel.Thread,
	runtime *ChatRuntime,
) (bool, []coremodel.ThreadMessage, error) {
	argv, ok := commandArgv(event.Payload.Text.Text)
	if !ok {
		return false, nil, nil
	}

	result := commandengine.Result{Text: "command error: unknown command"}
	if runtime != nil && runtime.MessageCommands != nil {
		runResult, runErr := runtime.MessageCommands.Run(
			ctx,
			commandengine.Request{
				Context: commandengine.Context{
					Source:   commandengine.SourceMessage,
					Actor:    messageCommandActor(event.Payload),
					ChatID:   chat.ID,
					ThreadID: thread.ID,
				},
			},
			argv,
		)
		if runErr != nil {
			result = commandengine.Result{
				Text: "command error: " + strings.TrimSpace(runErr.Error()),
			}
		} else {
			result = runResult
		}
	}

	text := strings.TrimSpace(result.Text)
	if text == "" {
		return true, nil, nil
	}

	message, err := b.storeAndRelayMessage(
		ctx,
		runtime,
		chat,
		thread,
		coremodel.ThreadMessage{
			Kind:       coremodel.MessageKindSystem,
			ActorID:    "ctgbot",
			ActorLabel: "ctgbot",
			Text:       text,
		},
		"ctgbot",
	)
	if err != nil {
		return true, nil, err
	}
	b.logf(
		"v5 message command handled chat=%s thread=%s argv=%q result_chars=%d",
		chat.ID,
		thread.ID,
		strings.Join(argv, " "),
		len(text),
	)
	return true, []coremodel.ThreadMessage{*message}, nil
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
