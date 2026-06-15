package server

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	hostbridgepolicy "github.com/bartdeboer/ctgbot/internal/hostbridgepolicy"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

func TestRunCommandRunnerExecutesAlias(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("default hostbridge aliass are empty on windows")
	}
	runner := &RunCommandRunner{ResolveAliases: StaticAliasResolver(nil), DefaultTimeoutSec: 5}
	result, err := runner.RunCommand(context.Background(), commandengine.Request{}, schemacommands.RunCommand{Command: "pwd"})
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if strings.TrimSpace(result.Text) == "" {
		t.Fatal("expected command output")
	}
}

func TestRunCommandRunnerRejectsUnknownCommand(t *testing.T) {
	runner := &RunCommandRunner{ResolveAliases: StaticAliasResolver(nil), DefaultTimeoutSec: 5}
	_, err := runner.RunCommand(context.Background(), commandengine.Request{}, schemacommands.RunCommand{Command: "definitely-not-allowed"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunCommandRunnerRegistersNewCommandHandler(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("default hostbridge aliass are empty on windows")
	}
	registry := commandengine.NewRegistry()
	runner := &RunCommandRunner{ResolveAliases: StaticAliasResolver(nil), DefaultTimeoutSec: 5}
	if err := RegisterRunCommandHandler(registry, runner); err != nil {
		t.Fatalf("RegisterRunCommandHandler() error = %v", err)
	}

	result, err := registry.Execute(context.Background(), commandengine.Request{
		Command: schemacommands.RunCommand{Command: "pwd"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.TrimSpace(result.Text) == "" {
		t.Fatal("expected command output")
	}
}

func TestRunCommandRunnerExecutesInstructionHiddenAlias(t *testing.T) {
	runner := &RunCommandRunner{ResolveAliases: StaticAliasResolver(map[string]Alias{
		"hidden-echo": {
			Name:                  "/bin/echo",
			Args:                  []string{"hidden-ok"},
			InstructionVisibility: hostbridgepolicy.AliasInstructionHidden,
		},
	}), DefaultTimeoutSec: 5}
	result, err := runner.RunCommand(context.Background(), commandengine.Request{}, schemacommands.RunCommand{Command: "hidden-echo"})
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if got, want := strings.TrimSpace(result.Text), "hidden-ok"; got != want {
		t.Fatalf("RunCommand() text = %q, want %q", got, want)
	}
}
