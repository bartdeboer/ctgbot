package broker

import (
	"testing"

	component "github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

func TestCompletionPromptFromMessages(t *testing.T) {
	messages := []coremodel.ThreadMessage{
		{Role: coremodel.MessageRoleSystem, Kind: coremodel.MessageKindMessage, Text: "system prompt"},
		{Direction: coremodel.MessageDirectionInbound, Role: coremodel.MessageRoleUser, Kind: coremodel.MessageKindMessage, Text: "hello"},
		{Direction: coremodel.MessageDirectionOutbound, Role: coremodel.MessageRoleAgent, Kind: coremodel.MessageKindMessage, Text: "hi"},
		{Direction: coremodel.MessageDirectionOutbound, Role: coremodel.MessageRoleAgent, Kind: coremodel.MessageKindProgress, Text: "reasoning preview"},
		{Direction: coremodel.MessageDirectionInbound, Text: "ignored missing role/kind"},
		{Kind: coremodel.MessageKindEvent, Text: "ignored event"},
		{Role: coremodel.MessageRoleUser, Kind: coremodel.MessageKindMessage, Text: "   "},
	}

	got := completionPromptFromMessages(messages)
	want := component.CompletionPrompt{
		Messages: []component.CompletionMessage{
			{Role: component.CompletionRoleSystem, Content: "system prompt"},
			{Role: component.CompletionRoleUser, Content: "hello"},
			{Role: component.CompletionRoleAssistant, Content: "hi"},
		},
	}

	if len(got.Messages) != len(want.Messages) {
		t.Fatalf("len(Messages) = %d, want %d: %#v", len(got.Messages), len(want.Messages), got.Messages)
	}
	for i := range want.Messages {
		if got.Messages[i] != want.Messages[i] {
			t.Fatalf("Messages[%d] = %#v, want %#v", i, got.Messages[i], want.Messages[i])
		}
	}
}
