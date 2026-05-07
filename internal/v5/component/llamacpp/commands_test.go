package llamacpp

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
)

func TestCommandDefinitions(t *testing.T) {
	component := &Component{}
	definitions := component.CommandDefinitions()
	want := map[string]string{
		"start":  "start",
		"stop":   "stop",
		"status": "status",
	}
	if len(definitions) != len(want) {
		t.Fatalf("len(CommandDefinitions) = %d, want %d", len(definitions), len(want))
	}
	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			t.Fatalf("definition %s invalid: %v", definition.ID(), err)
		}
		pattern, ok := want[definition.ID()]
		if !ok {
			t.Fatalf("unexpected definition id %q", definition.ID())
		}
		routes := definition.Routes()
		if len(routes) != 1 || commandengine.NormalizePattern(routes[0].Pattern) != pattern {
			t.Fatalf("definition %s routes = %#v, want %q", definition.ID(), routes, pattern)
		}
	}
}
