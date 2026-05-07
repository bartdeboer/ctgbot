package llamacpp

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
)

func TestCommandDefinitions(t *testing.T) {
	component := &Component{}
	definitions := component.CommandDefinitions()
	want := map[string]string{
		"llamacpp.start":  "llamacpp start",
		"llamacpp.stop":   "llamacpp stop",
		"llamacpp.status": "llamacpp status",
	}
	if len(definitions) != len(want) {
		t.Fatalf("len(CommandDefinitions) = %d, want %d", len(definitions), len(want))
	}
	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			t.Fatalf("definition %s invalid: %v", definition.ID, err)
		}
		pattern, ok := want[definition.ID]
		if !ok {
			t.Fatalf("unexpected definition id %q", definition.ID)
		}
		if len(definition.Routes) != 1 || commandengine.NormalizePattern(definition.Routes[0].Pattern) != pattern {
			t.Fatalf("definition %s routes = %#v, want %q", definition.ID, definition.Routes, pattern)
		}
	}
}
