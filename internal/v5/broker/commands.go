package broker

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	component "github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
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

	message, err := b.appendAndRelayMessage(
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

func messageCommandActor(payload messenger.InboundPayload) commandengine.Actor {
	actorID := strings.TrimSpace(payload.UserLabel)
	if payload.UserID != 0 {
		actorID = fmt.Sprintf("%d", payload.UserID)
	}
	if actorID == "" {
		actorID = "user"
	}
	return commandengine.Actor{
		ID:    actorID,
		Roles: []simplerbac.Role{simplerbac.RoleUser},
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
