package broker

import (
	"context"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
)

func (b *Broker) runStoredThreadTurn(
	ctx context.Context,
	runtime *ChatRuntime,
	chat coremodel.Chat,
	thread coremodel.Thread,
	turnInbound coremodel.ThreadMessage,
	options turnOptions,
) ([]coremodel.ThreadMessage, error) {
	turnRuntime := &agentTurnRuntime{
		ctx:     ctx,
		broker:  b,
		runtime: runtime,
		chat:    chat,
		thread:  thread,
	}

	var outbound []coremodel.ThreadMessage
	for _, agentBinding := range runtime.Agents {
		turnRuntime.componentID = agentBinding.ComponentID
		turnRuntime.lastText = ""
		final, err := b.runAgentTurn(ctx, agentBinding, chat, thread, turnInbound, turnRuntime)
		outbound = append(outbound, turnRuntime.outputs...)
		turnRuntime.outputs = nil
		if err != nil {
			return outbound, err
		}
		if final == nil || strings.TrimSpace(final.Text) == "" {
			continue
		}
		if strings.TrimSpace(final.Text) == turnRuntime.LastText() {
			continue
		}
		var attachments []message.Media
		if options.Mode == turnModeAudio {
			media, _, err := synthesizeTurnReply(ctx, runtime, final.Text)
			if err != nil {
				return outbound, err
			}
			if media != nil {
				attachments = append(attachments, *media)
			}
		}
		message, err := b.storeAndRelayMessageWithAttachments(ctx, runtime, chat, thread, *final, agentType(agentBinding), attachments)
		if err != nil {
			return outbound, err
		}
		outbound = append(outbound, *message)
	}

	return outbound, nil
}
