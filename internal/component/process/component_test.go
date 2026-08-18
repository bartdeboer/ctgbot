package process

import (
	"context"
	"reflect"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/buildassets"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
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
			pattern:       "go-generate",
			source:        commandengine.SourceMessage,
			visibleRoutes: []string{"go-generate", "go-generate"},
		},
		{
			pattern:       "go-generate",
			source:        commandengine.SourceCLI,
			visibleRoutes: []string{"go-generate"},
		},
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
			pattern:       "image build",
			source:        commandengine.SourceMessage,
			visibleRoutes: []string{"image build", "image build"},
		},
		{
			pattern:       "image build",
			source:        commandengine.SourceCLI,
			visibleRoutes: []string{"image build"},
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

func TestBuildQuitCommand(t *testing.T) {
	tests := []struct {
		name      string
		extra     []string
		want      quitCommand
		wantError bool
	}{
		{name: "normal", want: quitCommand{}},
		{name: "force", extra: []string{"force"}, want: quitCommand{Force: true}},
		{name: "force case insensitive", extra: []string{"FORCE"}, want: quitCommand{Force: true}},
		{name: "unknown argument", extra: []string{"now"}, wantError: true},
		{name: "too many arguments", extra: []string{"force", "now"}, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildQuitCommand(&clir.Request{Extra: tt.extra})
			if tt.wantError {
				if err == nil {
					t.Fatal("buildQuitCommand() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildQuitCommand() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("buildQuitCommand() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
