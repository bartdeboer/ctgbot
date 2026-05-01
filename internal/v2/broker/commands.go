package broker

import (
	"context"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/ctgbot/internal/v2/component"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

func (b *Broker) tryHandleCommand(ctx context.Context, event component.InboundEvent, chat coremodel.Chat, thread coremodel.Thread, bindings []coremodel.ChatComponent) (bool, EventOutcome, error) {
	argv, ok := commandArgv(event.Text)
	if !ok {
		return false, EventOutcome{}, nil
	}

	engine, err := b.components.CommandEngineForBindings(bindings, commandengine.SourceMessage)
	if err != nil {
		return true, EventOutcome{Command: true}, err
	}

	result, runErr := engine.Run(ctx, commandengine.Request{
		Context: commandengine.Context{
			Source:   commandengine.SourceMessage,
			Actor:    commandengine.Actor{ID: strings.TrimSpace(event.Actor.ID), Roles: b.rolesForEvent(ctx, event, chat)},
			ChatID:   chat.ID,
			ThreadID: thread.ID,
		},
	}, argv)
	if runErr != nil {
		result = commandengine.Result{Text: "command error: " + strings.TrimSpace(runErr.Error())}
	}

	message := coremodel.ThreadMessage{
		ChatID:       chat.ID,
		ThreadID:     thread.ID,
		Direction:    coremodel.DirectionOutbound,
		Kind:         coremodel.MessageKindSystem,
		SourceType:   "ctgbot",
		ActorID:      "ctgbot",
		ActorLabel:   "ctgbot",
		Text:         strings.TrimSpace(result.Text),
		MetadataJSON: metadataJSON(event.Metadata),
	}
	if message.Text == "" {
		return true, EventOutcome{Command: true}, nil
	}
	if err := b.storage.Messages().Append(ctx, &message); err != nil {
		return true, EventOutcome{Command: true}, err
	}
	b.logf("v2 command handled chat=%s thread=%s argv=%q result_chars=%d", chat.ID, thread.ID, strings.Join(argv, " "), len(message.Text))
	if err := b.relayOutbound(ctx, message, bindings); err != nil {
		return true, EventOutcome{Command: true, Outbound: []coremodel.ThreadMessage{message}}, err
	}
	return true, EventOutcome{Command: true, Outbound: []coremodel.ThreadMessage{message}}, nil
}

func (b *Broker) rolesForEvent(ctx context.Context, event component.InboundEvent, chat coremodel.Chat) []simplerbac.Role {
	if b != nil && b.RoleResolver != nil {
		roles := b.RoleResolver(ctx, event, chat)
		if len(roles) > 0 {
			return roles
		}
	}
	return []simplerbac.Role{simplerbac.RoleUser}
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
