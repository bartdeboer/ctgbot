package llamacppagent

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

func TestSandboxBaseURLRewritesLocalhost(t *testing.T) {
	t.Parallel()
	got := sandboxBaseURL("http://127.0.0.1:19080/v1")
	want := "http://host.docker.internal:19080/v1"
	if got != want {
		t.Fatalf("sandboxBaseURL() = %q, want %q", got, want)
	}
}

func TestToolloopMessagesIncludesConversationHistory(t *testing.T) {
	t.Parallel()
	history := []coremodel.ThreadMessage{
		{Kind: coremodel.MessageKindUser, Text: "Hello"},
		{Kind: coremodel.MessageKindAgent, Text: "Hi there"},
	}
	messages := toolloopMessages(history, coremodel.ThreadMessage{})
	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "Hello" {
		t.Fatalf("messages[0] = %#v", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Content != "Hi there" {
		t.Fatalf("messages[1] = %#v", messages[1])
	}
}
