package commandengine

import (
	"context"
	"strings"
	"testing"
)

func TestRegistryDispatchesByConcreteType(t *testing.T) {
	registry := NewRegistry()
	if err := Register[testCommand](registry, func(_ context.Context, req Request, cmd testCommand) (Result, error) {
		return Result{Text: cmd.Value}, nil
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	result, err := registry.Execute(context.Background(), Request{Command: testCommand{Value: "ok"}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Text != "ok" {
		t.Fatalf("result = %q, want ok", result.Text)
	}

	_, err = registry.Execute(context.Background(), Request{Command: &testCommand{Value: "ok"}})
	if err == nil || !strings.Contains(err.Error(), "unsupported command type") {
		t.Fatalf("Execute() pointer error = %v, want unsupported type", err)
	}
}

func TestRegistryDispatchesByCanonicalPattern(t *testing.T) {
	registry := NewRegistry()
	if err := RegisterPattern[testCommand](registry, "test status", func(_ context.Context, req Request, cmd testCommand) (Result, error) {
		if got, want := req.CanonicalPattern, "test status"; got != want {
			t.Fatalf("CanonicalPattern = %q, want %q", got, want)
		}
		return Result{Text: cmd.Value}, nil
	}); err != nil {
		t.Fatalf("RegisterPattern() error = %v", err)
	}

	result, err := registry.Execute(context.Background(), Request{
		CanonicalPattern: "test status",
		Command:          testCommand{Value: "ok"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := result.Text, "ok"; got != want {
		t.Fatalf("result = %q, want %q", got, want)
	}
}

func TestRegistryPatternPrefix(t *testing.T) {
	registry := NewRegistry().WithPatternPrefix("codex/work")
	if err := RegisterPattern[testCommand](registry, "status", func(_ context.Context, req Request, cmd testCommand) (Result, error) {
		return Result{Text: req.CanonicalPattern + "=" + cmd.Value}, nil
	}); err != nil {
		t.Fatalf("RegisterPattern() error = %v", err)
	}

	result, err := registry.Execute(context.Background(), Request{
		CanonicalPattern: "codex/work status",
		Command:          testCommand{Value: "ok"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := result.Text, "codex/work status=ok"; got != want {
		t.Fatalf("result = %q, want %q", got, want)
	}
}

func TestRegistryRejectsDuplicatePatternHandler(t *testing.T) {
	registry := NewRegistry()
	if err := RegisterPattern[testCommand](registry, "status", func(_ context.Context, req Request, cmd testCommand) (Result, error) {
		_, _, _ = req, cmd, registry
		return Result{}, nil
	}); err != nil {
		t.Fatalf("RegisterPattern() error = %v", err)
	}
	err := RegisterPattern[testCommand](registry, "status", func(_ context.Context, req Request, cmd testCommand) (Result, error) {
		_, _, _ = req, cmd, registry
		return Result{}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate command handler") {
		t.Fatalf("duplicate RegisterPattern() error = %v, want duplicate command handler", err)
	}
}

func TestRegistryPatternPrefixComposes(t *testing.T) {
	registry := NewRegistry().WithPatternPrefix("codex").WithPatternPrefix("work")
	if err := RegisterPattern[testCommand](registry, "status", func(_ context.Context, req Request, cmd testCommand) (Result, error) {
		return Result{Text: req.CanonicalPattern + "=" + cmd.Value}, nil
	}); err != nil {
		t.Fatalf("RegisterPattern() error = %v", err)
	}

	result, err := registry.Execute(context.Background(), Request{
		CanonicalPattern: "codex work status",
		Command:          testCommand{Value: "ok"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := result.Text, "codex work status=ok"; got != want {
		t.Fatalf("result = %q, want %q", got, want)
	}
}
