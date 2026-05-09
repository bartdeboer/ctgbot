package broker

import (
	"strings"

	component "github.com/bartdeboer/ctgbot/internal/component"
)

func prepareTurnInbound(inbound component.ResolvedInbound, prompt string) string {
	prompt = strings.TrimSpace(prompt)
	context := inbound.PromptContext
	if context == nil {
		return prompt
	}

	kind := strings.TrimSpace(context.Kind)
	if kind == "" {
		kind = "Inbound message"
	}

	lines := []string{"[" + kind + "]"}
	if label := strings.TrimSpace(context.FromLabel); label != "" {
		lines = append(lines, "From: "+label)
	}
	if actorID := strings.TrimSpace(context.FromID); actorID != "" {
		lines = append(lines, "Sender ID: "+actorID)
	}
	if replyHint := strings.TrimSpace(context.ReplyHint); replyHint != "" {
		lines = append(lines, "Reply path: "+replyHint)
	}
	if prompt != "" {
		lines = append(lines, "", "Message:", prompt)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
