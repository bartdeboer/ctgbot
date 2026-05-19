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
	voiceInput bool,
	detectedInputLanguage string,
) ([]coremodel.ThreadMessage, error) {
	turnRuntime := &agentTurnRuntime{
		ctx:                   ctx,
		broker:                b,
		runtime:               runtime,
		chat:                  chat,
		thread:                thread,
		voiceInput:            voiceInput,
		detectedInputLanguage: cleanLanguageCode(detectedInputLanguage),
	}
	turnRuntime.applyThreadVoiceConfig(thread)

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
		finalAlreadyRelayed := strings.TrimSpace(final.Text) == turnRuntime.LastText()
		if !finalAlreadyRelayed {
			message, err := b.storeAndRelayMessageWithAttachments(ctx, runtime, chat, thread, *final, agentType(agentBinding), nil)
			if err != nil {
				return outbound, err
			}
			outbound = append(outbound, *message)
		}
		if turnRuntime.voiceOutput {
			if err := b.relaySynthesizedTurnReply(ctx, runtime, turnRuntime, final.Text); err != nil {
				return outbound, err
			}
		}
	}

	return outbound, nil
}
