package broker

import (
	"context"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/ctgbot/internal/v2/component"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

func (b *Broker) tryHandleCommand(ctx context.Context, event component.InboundEvent, chat coremodel.Chat, thread coremodel.Thread, runtime *ChatRuntime) (bool, EventOutcome, error) {
	argv, ok := commandArgv(event.Text)
	if !ok {
		return false, EventOutcome{}, nil
	}
	if runtime == nil || runtime.MessageCommands == nil {
		return true, EventOutcome{Command: true}, nil
	}

	// Chat commands arrive as raw message text, so this path parses argv
	// through the cached message command router. Sandbox/hostbridge commands
	// are already parsed by the hostbridge client and should use
	// ChatRuntime.AgentCommands.Execute instead.
	result, runErr := runtime.MessageCommands.Run(ctx, commandengine.Request{
		Context: commandengine.Context{
			Source:   commandengine.SourceMessage,
			Actor:    commandengine.Actor{ID: strings.TrimSpace(event.Actor.ID), Roles: effectiveActorRoles(event.Actor.Roles)},
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
	if err := b.relayOutbound(ctx, message, runtime); err != nil {
		return true, EventOutcome{Command: true, Outbound: []coremodel.ThreadMessage{message}}, err
	}
	return true, EventOutcome{Command: true, Outbound: []coremodel.ThreadMessage{message}}, nil
}

func effectiveActorRoles(roles []simplerbac.Role) []simplerbac.Role {
	if len(roles) == 0 {
		return []simplerbac.Role{simplerbac.RoleUser}
	}
	return append([]simplerbac.Role(nil), roles...)
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
