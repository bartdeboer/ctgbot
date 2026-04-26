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
