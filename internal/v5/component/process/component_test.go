package process

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
)

func TestProcessCommandDefinitions(t *testing.T) {
	component := New(nil)
	definitions := component.CommandDefinitions()
	want := []struct {
		pattern       string
		source        commandengine.Source
		visibleRoutes []string
	}{
		{
			pattern:       "install",
			source:        commandengine.SourceMessage,
			visibleRoutes: []string{"install", "install"},
		},
		{
			pattern:       "install",
			source:        commandengine.SourceCLI,
			visibleRoutes: []string{"install"},
		},
		{
			pattern:       "upgrade",
			source:        commandengine.SourceMessage,
			visibleRoutes: []string{"upgrade", "upgrade"},
		},
		{
			pattern:       "upgrade",
			source:        commandengine.SourceCLI,
			visibleRoutes: []string{"upgrade"},
		},
		{
			pattern:       "quit",
			source:        commandengine.SourceMessage,
			visibleRoutes: []string{"quit", "quit"},
		},
		{
			pattern:       "quit",
			source:        commandengine.SourceCLI,
			visibleRoutes: []string{"quit"},
		},
	}
	if len(definitions) != len(want) {
		t.Fatalf("len(CommandDefinitions) = %d, want %d", len(definitions), len(want))
	}
	for defIndex, definition := range definitions {
		got := want[defIndex]
		if pattern := definition.CanonicalPattern(); pattern != got.pattern {
			t.Fatalf("definition[%d] pattern = %q, want %q", defIndex, pattern, got.pattern)
		}
		if len(definition.Sources) != 1 || definition.Sources[0] != got.source {
			t.Fatalf("definition[%d] sources = %#v, want [%q]", defIndex, definition.Sources, got.source)
		}
		routes := definition.Routes()
		if len(routes) != len(got.visibleRoutes) {
			t.Fatalf("definition[%d] route count = %d, want %d", defIndex, len(routes), len(got.visibleRoutes))
		}
		for routeIndex, route := range routes {
			if gotPattern := commandengine.NormalizePattern(route.Pattern); gotPattern != got.visibleRoutes[routeIndex] {
				t.Fatalf("definition[%d] route[%d] = %q, want %q", defIndex, routeIndex, gotPattern, got.visibleRoutes[routeIndex])
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
