package llamacpp

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
)

func TestBrokerMessagesToChat(t *testing.T) {
	messages := []coremodel.ThreadMessage{
		{Kind: coremodel.MessageKindSystem, Text: "system prompt"},
		{Direction: coremodel.MessageDirectionInbound, Kind: coremodel.MessageKindUser, Text: "hello"},
		{Direction: coremodel.MessageDirectionOutbound, Kind: coremodel.MessageKindAgent, Text: "hi"},
		{Direction: coremodel.MessageDirectionInbound, Text: "fallback user"},
		{Direction: coremodel.MessageDirectionOutbound, Text: "fallback assistant"},
		{Kind: coremodel.MessageKindUser, Text: "   "},
	}

	got := brokerMessagesToChat(messages)
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
