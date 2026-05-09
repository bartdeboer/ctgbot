package broker

import (
	"context"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

func (b *Broker) runStoredThreadTurn(
	ctx context.Context,
	runtime *ChatRuntime,
	chat coremodel.Chat,
	thread coremodel.Thread,
	turnInbound coremodel.ThreadMessage,
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
		message, err := b.storeAndRelayMessage(ctx, runtime, chat, thread, *final, agentType(agentBinding))
		if err != nil {
			return outbound, err
		}
		outbound = append(outbound, *message)
	}

	return outbound, nil
}
