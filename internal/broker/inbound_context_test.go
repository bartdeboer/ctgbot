package broker

import (
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/component"
)

func TestPrepareTurnInboundWithoutPromptContextReturnsPrompt(t *testing.T) {
	prompt := "hello"
	got := prepareTurnInbound(component.ResolvedInbound{}, prompt)
	if got != prompt {
		t.Fatalf("prepareTurnInbound() = %q, want %q", got, prompt)
	}
}

func TestPrepareTurnInboundWithPromptContextInjectsSourceMetadata(t *testing.T) {
	got := prepareTurnInbound(component.ResolvedInbound{
		PromptContext: &component.InboundPromptContext{
			Kind:      "Internal thread message",
			FromLabel: "source thread",
			FromID:    "thread:123",
			ReplyHint: "hostbridge thread 123 message send",
		},
	}, "hello")
	for _, want := range []string{
		"[Internal thread message]",
		"From: source thread",
		"Sender ID: thread:123",
		"Reply path: hostbridge thread 123 message send",
		"Message:\nhello",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prepareTurnInbound() = %q, want to contain %q", got, want)
		}
	}
}
