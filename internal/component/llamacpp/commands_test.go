package llamacpp

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
)

func TestCommandDefinitions(t *testing.T) {
	component := &Component{}
	definitions := component.CommandDefinitions()
	want := map[string]string{
		"start":                             "start",
		"stop":                              "stop",
		"status":                            "status",
		"completion <prompt>":               "completion <prompt>",
		"embed <text>":                      "embed <text>",
		"model list":                        "model list",
		"model install <name> <url>":        "model install <name> <url>",
		"model register <name> <path>":      "model register <name> <path>",
		"model <model> completion <prompt>": "model <model> completion <prompt>",
		"model <model> embed <text>":        "model <model> embed <text>",
	}
	if len(definitions) != len(want) {
		t.Fatalf("len(CommandDefinitions) = %d, want %d", len(definitions), len(want))
	}
	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			t.Fatalf("definition %s invalid: %v", definition.CanonicalPattern(), err)
		}
		pattern, ok := want[definition.CanonicalPattern()]
		if !ok {
			t.Fatalf("unexpected canonical pattern %q", definition.CanonicalPattern())
		}
		routes := definition.Routes()
		if len(routes) != 1 || commandengine.NormalizePattern(routes[0].Pattern) != pattern {
			t.Fatalf("definition %s routes = %#v, want %q", definition.CanonicalPattern(), routes, pattern)
		}
	}
}
