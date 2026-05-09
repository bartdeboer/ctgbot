package broker

import (
	"strings"

	component "github.com/bartdeboer/ctgbot/internal/component"
)

func prepareTurnInbound(inbound component.ResolvedInbound, prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if strings.TrimSpace(inbound.Payload.ProviderType) != "thread" {
		return prompt
	}

	actor := inbound.Payload.ResolvedActor()
	sourceThreadID := inboundMetadataValue(inbound.Metadata, "source_thread_id")

	lines := []string{"[Internal thread message]"}
	if label := strings.TrimSpace(actor.Label); label != "" {
		lines = append(lines, "From: "+label)
	}
	if actorID := strings.TrimSpace(actor.ID); actorID != "" {
		lines = append(lines, "Sender ID: "+actorID)
	}
	if sourceThreadID != "" {
		lines = append(lines, "Reply path: hostbridge thread "+sourceThreadID+" message send <message>")
	}
	if prompt != "" {
		lines = append(lines, "", "Message:", prompt)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func inboundMetadataValue(metadata []string, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	prefix := key + "="
	for _, line := range metadata {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(line, prefix))
	}
	return ""
}
