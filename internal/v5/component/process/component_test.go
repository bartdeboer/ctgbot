package process

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
)

func TestProcessCommandDefinitions(t *testing.T) {
	component := New(nil)
	definitions := component.CommandDefinitions()
	want := map[string]struct {
		source        commandengine.Source
		visibleRoutes []string
	}{
		"install": {
			source:        commandengine.SourceMessage,
			visibleRoutes: []string{"install", "install"},
		},
		"process install": {
			source:        commandengine.SourceCLI,
			visibleRoutes: []string{"process install"},
		},
		"upgrade": {
			source:        commandengine.SourceMessage,
			visibleRoutes: []string{"upgrade", "upgrade"},
		},
		"process upgrade": {
			source:        commandengine.SourceCLI,
			visibleRoutes: []string{"process upgrade"},
		},
		"quit": {
			source:        commandengine.SourceMessage,
			visibleRoutes: []string{"quit", "quit"},
		},
		"process quit": {
			source:        commandengine.SourceCLI,
			visibleRoutes: []string{"process quit"},
		},
	}
	if len(definitions) != len(want) {
		t.Fatalf("len(CommandDefinitions) = %d, want %d", len(definitions), len(want))
	}
	for _, definition := range definitions {
		got, ok := want[definition.CanonicalPattern()]
		if !ok {
			t.Fatalf("unexpected canonical pattern %q", definition.CanonicalPattern())
		}
		if len(definition.Sources) != 1 || definition.Sources[0] != got.source {
			t.Fatalf("definition %q sources = %#v, want [%q]", definition.CanonicalPattern(), definition.Sources, got.source)
		}
		routes := definition.Routes()
		if len(routes) != len(got.visibleRoutes) {
			t.Fatalf("definition %q route count = %d, want %d", definition.CanonicalPattern(), len(routes), len(got.visibleRoutes))
		}
		for i, route := range routes {
			if gotPattern := commandengine.NormalizePattern(route.Pattern); gotPattern != got.visibleRoutes[i] {
				t.Fatalf("definition %q route[%d] = %q, want %q", definition.CanonicalPattern(), i, gotPattern, got.visibleRoutes[i])
			}
		}
	}
}

func TestProcessCommandDefinitionsDoNotExposeHostbridgeRoutes(t *testing.T) {
	component := New(nil)
	for _, definition := range component.CommandDefinitions() {
		for _, source := range definition.Sources {
			if source == commandengine.SourceHostbridge {
				t.Fatalf("definition %q unexpectedly exposes hostbridge source", definition.CanonicalPattern())
			}
		}
	}
}
