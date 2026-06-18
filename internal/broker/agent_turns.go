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
	prompt string,
	voiceInput bool,
	detectedInputLanguage string,
	inputFiles []turnInputFile,
) ([]coremodel.ThreadMessage, error) {
	turnRuntime := &agentTurnRuntime{
		ctx:                   ctx,
		broker:                b,
		runtime:               runtime,
		chat:                  chat,
		thread:                thread,
		voiceInput:            voiceInput,
		detectedInputLanguage: cleanLanguageCode(detectedInputLanguage),
		inputFiles:            append([]turnInputFile(nil), inputFiles...),
	}
	turnRuntime.applyThreadVoiceConfig(thread)

	var outbound []coremodel.ThreadMessage
	for _, agentBinding := range runtime.Agents {
		turnRuntime.componentID = agentBinding.ComponentID
		turnRuntime.lastText = ""
		result, err := b.runAgentTurn(ctx, agentBinding, chat, thread, turnInbound, prompt, turnRuntime)
		outbound = append(outbound, turnRuntime.outputs...)
		turnRuntime.outputs = nil
		if err != nil {
			return outbound, err
		}
		if result == nil {
			continue
		}
		if result.Relay != nil && strings.TrimSpace(result.Relay.Text) != "" {
			payload := turnRuntime.applyTurnOutputDefaults(messagePayload(result.Relay.Text))
			if err := b.relayOnlyMessageWithPayloadText(ctx, runtime, chat, thread, *result.Relay, payload.Text); err != nil {
				return outbound, err
			}
		}
		final := result.Final
		if final == nil || strings.TrimSpace(final.Text) == "" {
			continue
		}
		finalAlreadyRelayed := strings.TrimSpace(final.Text) == turnRuntime.LastText()
		if !finalAlreadyRelayed {
			payload := turnRuntime.applyTurnOutputDefaults(messagePayload(final.Text))
			message, err := b.storeAndRelayMessageWithPayloadText(ctx, runtime, chat, thread, *final, agentType(agentBinding), payload.Text, nil)
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

func messagePayload(text string) message.OutboundPayload {
	return message.OutboundPayload{Text: message.TextMessage{Text: text}}
}
