package broker

import (
	"testing"

	component "github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

func TestCompletionPromptFromMessages(t *testing.T) {
	messages := []coremodel.ThreadMessage{
		{Kind: coremodel.MessageKindSystem, Text: "system prompt"},
		{Direction: coremodel.MessageDirectionInbound, Kind: coremodel.MessageKindUser, Text: "hello"},
		{Direction: coremodel.MessageDirectionOutbound, Kind: coremodel.MessageKindAgent, Text: "hi"},
		{Direction: coremodel.MessageDirectionInbound, Text: "fallback user"},
		{Direction: coremodel.MessageDirectionOutbound, Text: "fallback assistant"},
		{Kind: coremodel.MessageKindEvent, Text: "ignored event"},
		{Kind: coremodel.MessageKindUser, Text: "   "},
	}

	got := completionPromptFromMessages(messages)
	want := component.CompletionPrompt{
		Messages: []component.CompletionMessage{
			{Role: component.CompletionRoleSystem, Content: "system prompt"},
			{Role: component.CompletionRoleUser, Content: "hello"},
			{Role: component.CompletionRoleAssistant, Content: "hi"},
			{Role: component.CompletionRoleUser, Content: "fallback user"},
			{Role: component.CompletionRoleAssistant, Content: "fallback assistant"},
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
