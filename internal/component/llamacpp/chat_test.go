package llamacpp

import (
	"testing"

	component "github.com/bartdeboer/ctgbot/internal/component"
)

func TestCompletionPromptToChat(t *testing.T) {
	prompt := component.CompletionPrompt{
		Messages: []component.CompletionMessage{
			{Role: component.CompletionRoleSystem, Content: "system prompt"},
			{Role: component.CompletionRoleUser, Content: "hello"},
			{Role: component.CompletionRoleAssistant, Content: "hi"},
			{Role: component.CompletionRoleUser, Content: "fallback user"},
			{Role: component.CompletionRoleAssistant, Content: "fallback assistant"},
			{Role: component.CompletionRoleUser, Content: "   "},
		},
	}

	got := completionPromptToChat(prompt)
	want := []chatMessage{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
		{Role: "user", Content: "fallback user"},
		{Role: "assistant", Content: "fallback assistant"},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("message[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
