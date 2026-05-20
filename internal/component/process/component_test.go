package process

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/buildassets"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
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
			pattern:       "upgrade all",
			source:        commandengine.SourceMessage,
			visibleRoutes: []string{"upgrade all", "upgrade all"},
		},
		{
			pattern:       "upgrade all",
			source:        commandengine.SourceCLI,
			visibleRoutes: []string{"upgrade all"},
		},
		{
			pattern:       "image list",
			source:        commandengine.SourceMessage,
			visibleRoutes: []string{"image list", "image list"},
		},
		{
			pattern:       "image list",
			source:        commandengine.SourceCLI,
			visibleRoutes: []string{"image list"},
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
		{
			pattern:       "version",
			source:        commandengine.SourceMessage,
			visibleRoutes: []string{"version", "version"},
		},
		{
			pattern:       "version",
			source:        commandengine.SourceCLI,
			visibleRoutes: []string{"version"},
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

func TestProcessVersionCommandAllowsUser(t *testing.T) {
	engine, err := commandset.NewBoundEngineForSource(commandengine.SourceMessage, []commandset.BoundSurface{{
		Surface:       New(nil),
		ComponentRef:  Type,
		ComponentType: Type,
	}})
	if err != nil {
		t.Fatalf("NewBoundEngineForSource() error = %v", err)
	}
	result, err := engine.Run(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Source: commandengine.SourceMessage,
			Actor:  commandengine.Actor{ID: "user", Roles: []simplerbac.Role{simplerbac.RoleUser}},
		},
	}, []string{"version"})
	if err != nil {
		t.Fatalf("Run(version) error = %v", err)
	}
	if got, want := result.Text, buildassets.Version(); got != want {
		t.Fatalf("version text = %q, want %q", got, want)
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
