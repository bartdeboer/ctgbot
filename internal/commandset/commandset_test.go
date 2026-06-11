package commandset

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type testCommandSurface struct{}

func (c *testCommandSurface) Type() string { return "codex" }
func (c *testCommandSurface) UsesLocalCommandRoutes() bool {
	return true
}
func (c *testCommandSurface) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{{
		Pattern: "status",
		Help:    "Show status",
		Build: func(req *clir.Request) (any, error) {
			_ = req
			return testCommand{}, nil
		},
		Sources: []commandengine.Source{commandengine.SourceMessage},
	}}
}
func (c *testCommandSurface) RegisterCommandHandlers(registry *commandengine.Registry) error {
	return commandengine.RegisterPattern[testCommand](registry, "status", func(ctx context.Context, req commandengine.Request, cmd testCommand) (commandengine.Result, error) {
		_, _ = ctx, cmd
		return commandengine.Result{Text: req.CanonicalPattern}, nil
	})
}

type testCommand struct{}

type conflictingAliasSurface struct {
	componentType string
	alias         string
}

func (c conflictingAliasSurface) Type() string { return c.componentType }
func (c conflictingAliasSurface) UsesLocalCommandRoutes() bool {
	return true
}
func (c conflictingAliasSurface) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{{
		Pattern: "hidden",
		Help:    "Hidden command",
		Build: func(req *clir.Request) (any, error) {
			_ = req
			return testCommand{}, nil
		},
		Sources: []commandengine.Source{commandengine.SourceMessage},
		Hidden:  true,
		Aliases: []commandengine.Route{{
			Pattern:  c.alias,
			Absolute: true,
		}},
	}}
}
func (c conflictingAliasSurface) RegisterCommandHandlers(registry *commandengine.Registry) error {
	return nil
}

func TestNewBoundEngineForSourceRejectsConflictingAbsoluteAliases(t *testing.T) {
	_, err := NewBoundEngineForSource(commandengine.SourceMessage, []BoundSurface{
		{Surface: conflictingAliasSurface{componentType: "one", alias: "thread wake list"}, ComponentRef: "one", ComponentType: "one"},
		{Surface: conflictingAliasSurface{componentType: "two", alias: "thread wake list"}, ComponentRef: "two", ComponentType: "two"},
	})
	if err == nil {
		t.Fatal("NewBoundEngineForSource() error = nil, want duplicate route")
	}
}

func TestNewBoundEngineForSourceSupportsUniqueTypeShorthand(t *testing.T) {
	engine, err := NewBoundEngineForSource(commandengine.SourceMessage, []BoundSurface{{
		Surface:       &testCommandSurface{},
		ComponentRef:  "codex/work",
		ComponentType: "codex",
	}})
	if err != nil {
		t.Fatalf("NewBoundEngineForSource() error = %v", err)
	}
	base := commandengine.Request{Context: commandengine.Context{Source: commandengine.SourceMessage}}

	result, err := engine.Run(context.Background(), base, []string{"codex", "status"})
	if err != nil {
		t.Fatalf("Run(codex status) error = %v", err)
	}
	if got, want := result.Text, "codex/work status"; got != want {
		t.Fatalf("Run(codex status) = %q, want %q", got, want)
	}

	result, err = engine.Run(context.Background(), base, []string{"codex/work", "status"})
	if err != nil {
		t.Fatalf("Run(codex/work status) error = %v", err)
	}
	if got, want := result.Text, "codex/work status"; got != want {
		t.Fatalf("Run(codex/work status) = %q, want %q", got, want)
	}
}

func TestNewBoundEngineForSourceRequiresFullRefForMultipleInstances(t *testing.T) {
	engine, err := NewBoundEngineForSource(commandengine.SourceMessage, []BoundSurface{
		{Surface: &testCommandSurface{}, ComponentRef: "codex/work", ComponentType: "codex"},
		{Surface: &testCommandSurface{}, ComponentRef: "codex/personal", ComponentType: "codex"},
	})
	if err != nil {
		t.Fatalf("NewBoundEngineForSource() error = %v", err)
	}
	base := commandengine.Request{Context: commandengine.Context{Source: commandengine.SourceMessage}}

	if _, err := engine.Run(context.Background(), base, []string{"codex", "status"}); err == nil {
		t.Fatal("Run(codex status) error = nil, want no matching command")
	}

	result, err := engine.Run(context.Background(), base, []string{"codex/work", "status"})
	if err != nil {
		t.Fatalf("Run(codex/work status) error = %v", err)
	}
	if got, want := result.Text, "codex/work status"; got != want {
		t.Fatalf("Run(codex/work status) = %q, want %q", got, want)
	}

	result, err = engine.Run(context.Background(), base, []string{"codex/personal", "status"})
	if err != nil {
		t.Fatalf("Run(codex/personal status) error = %v", err)
	}
	if got, want := result.Text, "codex/personal status"; got != want {
		t.Fatalf("Run(codex/personal status) = %q, want %q", got, want)
	}
}

func TestInstructionRoutePatternsFiltersVisibilityAndPermissions(t *testing.T) {
	definitions := []commandengine.Definition{
		testInstructionDefinition("hidden", commandengine.InstructionHidden, simplerbac.Any(simplerbac.RoleAgent)),
		testInstructionDefinition("discoverable leaf", "", simplerbac.Any(simplerbac.RoleAgent)),
		testInstructionDefinition("important leaf", commandengine.InstructionImportant, simplerbac.Any(simplerbac.RoleAgent)),
		testInstructionDefinition("essential leaf", commandengine.InstructionEssential, simplerbac.Any(simplerbac.RoleAgent)),
		testInstructionDefinition("root only", commandengine.InstructionEssential, simplerbac.Any(simplerbac.RoleRoot)),
		testInstructionDefinition("tool help", "", simplerbac.Any(simplerbac.RoleAgent)),
	}

	got := InstructionRoutePatterns(definitions, coremodel.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}})
	for _, want := range []string{"important leaf", "essential leaf", "tool help"} {
		if !containsPattern(got, want) {
			t.Fatalf("InstructionRoutePatterns() missing %q in %#v", want, got)
		}
	}
	for _, notWant := range []string{"hidden", "discoverable leaf", "root only"} {
		if containsPattern(got, notWant) {
			t.Fatalf("InstructionRoutePatterns() unexpectedly contains %q in %#v", notWant, got)
		}
	}
}

func TestInstructionRoutePatternsDefaultsUnsetVisibilityToDiscoverable(t *testing.T) {
	definition := testInstructionDefinition("plain command", "", simplerbac.Any(simplerbac.RoleAgent))
	if got, want := definition.InstructionVisibilityOrDefault(), commandengine.InstructionDiscoverable; got != want {
		t.Fatalf("InstructionVisibilityOrDefault() = %q, want %q", got, want)
	}
	patterns := InstructionRoutePatterns([]commandengine.Definition{definition}, coremodel.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}})
	if len(patterns) != 0 {
		t.Fatalf("InstructionRoutePatterns() = %#v, want empty discoverable leaf list", patterns)
	}
}

func TestInstructionRoutePatternsAddsScopedHelpForDiscoverableFamilies(t *testing.T) {
	definitions := []commandengine.Definition{
		testInstructionDefinition("codex status", commandengine.InstructionImportant, simplerbac.Any(simplerbac.RoleAgent)),
		testInstructionDefinition("codex config get <key>", "", simplerbac.Any(simplerbac.RoleAgent)),
		testInstructionDefinition("send <text>", commandengine.InstructionEssential, simplerbac.Any(simplerbac.RoleAgent)),
	}

	patterns := InstructionRoutePatterns(definitions, coremodel.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}})
	for _, want := range []string{"codex status", "codex help", "send <text>"} {
		if !containsPattern(patterns, want) {
			t.Fatalf("InstructionRoutePatterns() missing %q in %#v", want, patterns)
		}
	}
	if containsPattern(patterns, "codex config get <key>") {
		t.Fatalf("InstructionRoutePatterns() unexpectedly contains discoverable leaf in %#v", patterns)
	}
	if containsPattern(patterns, "send help") {
		t.Fatalf("InstructionRoutePatterns() unexpectedly adds help for single root command in %#v", patterns)
	}
}

func testInstructionDefinition(pattern string, visibility commandengine.InstructionVisibility, policy simplerbac.Rule) commandengine.Definition {
	return commandengine.Definition{
		Pattern:               pattern,
		Help:                  pattern,
		Build:                 func(req *clir.Request) (any, error) { _ = req; return testCommand{}, nil },
		Sources:               []commandengine.Source{commandengine.SourceHostbridge},
		Policy:                policy,
		InstructionVisibility: visibility,
	}
}

func containsPattern(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestInstructionFamilyDescriptionsFiltersPolicyAndRootPatterns(t *testing.T) {
	descriptions := []commandengine.Description{
		{Pattern: "theater", Help: "Theater commands", Policy: simplerbac.Any(simplerbac.RoleAgent)},
		{Pattern: "root", Help: "Root commands", Policy: simplerbac.Any(simplerbac.RoleRoot)},
		{Pattern: "thread message", Help: "Nested commands", Policy: simplerbac.Any(simplerbac.RoleAgent)},
		{Pattern: "hidden", Help: "Hidden commands", Policy: simplerbac.Any(simplerbac.RoleAgent), Hidden: true},
	}
	got := InstructionFamilyDescriptions(descriptions, coremodel.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}})
	if got["theater"] != "Theater commands" {
		t.Fatalf("theater description = %q", got["theater"])
	}
	for _, notWant := range []string{"root", "thread message", "hidden"} {
		if _, ok := got[notWant]; ok {
			t.Fatalf("InstructionFamilyDescriptions() unexpectedly contains %q in %#v", notWant, got)
		}
	}
}
