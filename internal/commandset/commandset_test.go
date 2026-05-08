package commandset

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
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
