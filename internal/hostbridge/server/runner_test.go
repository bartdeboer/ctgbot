package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

func TestRunCommandRunnerForwardsStdinByteForByte(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires /bin/cat")
	}
	stdin := "line one\n$(touch should-not-run)\n`backticks` \\\"quotes\\\" \\\\ tail\x00\x80\xff\n"
	runner := &RunCommandRunner{ResolveAliases: StaticAliasResolver(map[string]Alias{
		"cat": {Name: "/bin/cat", StdinMaxBytes: int64(len(stdin))},
	})}
	result, err := runner.RunCommand(context.Background(), commandengine.Request{Stdin: stdin}, schemacommands.RunCommand{Command: "cat"})
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if result.Execution == nil {
		t.Fatal("RunCommand() execution = nil")
	}
	if result.Execution.Stdout != stdin {
		t.Fatalf("stdout = %q, want exact stdin %q", result.Execution.Stdout, stdin)
	}
}

func TestRunCommandRunnerPreservesProcessOutcome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires executable shell script")
	}
	tests := []struct {
		name     string
		exitCode int
	}{
		{name: "success", exitCode: 0},
		{name: "typed nonzero", exitCode: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			executable := writeExecutable(t, "#!/bin/sh\nprintf '{\"status\":\"degraded\"}\\n'\nprintf 'health diagnostic\\n' >&2\nexit \"$1\"\n")
			runner := &RunCommandRunner{ResolveAliases: StaticAliasResolver(map[string]Alias{
				"probe": {Name: executable, Args: []string{fmt.Sprint(tc.exitCode)}},
			})}
			result, err := runner.RunCommand(context.Background(), commandengine.Request{}, schemacommands.RunCommand{Command: "probe"})
			if tc.exitCode == 0 && err != nil {
				t.Fatalf("RunCommand() error = %v", err)
			}
			if tc.exitCode != 0 && (err == nil || !strings.Contains(err.Error(), "exit status 2: health diagnostic")) {
				t.Fatalf("RunCommand() error = %v, want typed child exit", err)
			}
			if result.Execution == nil {
				t.Fatal("RunCommand() execution = nil")
			}
			if got, want := result.Execution.Stdout, "{\"status\":\"degraded\"}\n"; got != want {
				t.Fatalf("stdout = %q, want %q", got, want)
			}
			if got, want := result.Execution.Stderr, "health diagnostic\n"; got != want {
				t.Fatalf("stderr = %q, want %q", got, want)
			}
			if got := result.Execution.ExitCode; got != tc.exitCode {
				t.Fatalf("exit code = %d, want %d", got, tc.exitCode)
			}
		})
	}
}

func TestRunCommandRunnerPreservesSignaledProcessOutcome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX signals")
	}
	executable := writeExecutable(t, "#!/bin/sh\nprintf 'before signal\\n'\nprintf 'signal diagnostic\\n' >&2\nkill -TERM $$\n")
	runner := &RunCommandRunner{ResolveAliases: StaticAliasResolver(map[string]Alias{
		"probe": {Name: executable},
	})}
	result, err := runner.RunCommand(context.Background(), commandengine.Request{}, schemacommands.RunCommand{Command: "probe"})
	if err == nil || !strings.Contains(err.Error(), "exit status 143: signal diagnostic") {
		t.Fatalf("RunCommand() error = %v, want signaled child outcome", err)
	}
	if result.Execution == nil {
		t.Fatal("RunCommand() execution = nil")
	}
	if got, want := result.Execution.Stdout, "before signal\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got, want := result.Execution.Stderr, "signal diagnostic\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
	if got, want := result.Execution.ExitCode, 143; got != want {
		t.Fatalf("exit code = %d, want %d", got, want)
	}
}

func TestRunCommandRunnerDeniesStdinBeforeProcessStart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires executable shell script")
	}
	marker := filepath.Join(t.TempDir(), "started")
	executable := writeExecutable(t, "#!/bin/sh\ntouch \"$1\"\n")
	runner := &RunCommandRunner{ResolveAliases: StaticAliasResolver(map[string]Alias{
		"probe": {Name: executable, Args: []string{marker}},
	})}
	secret := "must-not-appear-in-errors"
	_, err := runner.RunCommand(context.Background(), commandengine.Request{Stdin: secret}, schemacommands.RunCommand{Command: "probe"})
	if err == nil || !strings.Contains(err.Error(), "does not allow stdin") {
		t.Fatalf("RunCommand() error = %v, want stdin denied", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("RunCommand() error disclosed stdin: %v", err)
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("process started despite denied stdin: stat error = %v", statErr)
	}
}

func TestRunCommandRunnerRejectsAliasStdinOverLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires /bin/cat")
	}
	runner := &RunCommandRunner{ResolveAliases: StaticAliasResolver(map[string]Alias{
		"cat": {Name: "/bin/cat", StdinMaxBytes: 4},
	})}
	_, err := runner.RunCommand(context.Background(), commandengine.Request{Stdin: "12345"}, schemacommands.RunCommand{Command: "cat"})
	if err == nil || !strings.Contains(err.Error(), "exceeds 4 bytes") {
		t.Fatalf("RunCommand() error = %v, want alias stdin limit", err)
	}
}

func TestRunCommandRunnerCancellationTerminatesChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires /bin/sleep")
	}
	runner := &RunCommandRunner{ResolveAliases: StaticAliasResolver(map[string]Alias{
		"sleep": {Name: "/bin/sleep", Args: []string{"10"}},
	}), DefaultTimeoutSec: 30}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := runner.RunCommand(ctx, commandengine.Request{}, schemacommands.RunCommand{Command: "sleep"})
	if err == nil {
		t.Fatal("RunCommand() error = nil, want cancellation")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("cancellation took %s", elapsed)
	}
}

func TestRunCommandRunnerDefaultsToOneMinute(t *testing.T) {
	runner := &RunCommandRunner{}
	if got := runner.defaultTimeoutSec(0); got != 60 {
		t.Fatalf("defaultTimeoutSec(0) = %d, want 60", got)
	}
}

func writeExecutable(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "probe.sh")
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	return path
}

func TestRunCommandRunnerExecutesAlias(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("default hostbridge aliass are empty on windows")
	}
	runner := &RunCommandRunner{ResolveAliases: StaticAliasResolver(nil), DefaultTimeoutSec: 5}
	result, err := runner.RunCommand(context.Background(), commandengine.Request{}, schemacommands.RunCommand{Command: "pwd"})
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if result.Execution == nil || strings.TrimSpace(result.Execution.Stdout) == "" {
		t.Fatal("expected command output")
	}
}

func TestRunCommandRunnerUsesAllowedCWDWithoutForwardingIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires /bin/pwd")
	}
	repo := t.TempDir()
	runner := &RunCommandRunner{ResolveAliases: StaticAliasResolver(map[string]Alias{
		"repo-pwd": {Name: "/bin/pwd", AllowedCWDs: []string{repo}},
	})}
	result, err := runner.RunCommand(context.Background(), commandengine.Request{}, schemacommands.RunCommand{
		Command: "repo-pwd",
		Args:    []string{"--cwd", repo},
	})
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if result.Execution == nil {
		t.Fatal("RunCommand() execution = nil")
	}
	if got, want := strings.TrimSpace(result.Execution.Stdout), repo; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
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
	if result.Execution == nil || strings.TrimSpace(result.Execution.Stdout) == "" {
		t.Fatal("expected command output")
	}
}

func TestRunCommandRunnerReturnsSpawnFailureAsOrdinaryError(t *testing.T) {
	runner := &RunCommandRunner{ResolveAliases: StaticAliasResolver(map[string]Alias{
		"missing": {Name: filepath.Join(t.TempDir(), "not-installed")},
	})}
	result, err := runner.RunCommand(context.Background(), commandengine.Request{}, schemacommands.RunCommand{Command: "missing"})
	if err == nil {
		t.Fatal("RunCommand() error = nil, want spawn failure")
	}
	if result.Execution != nil {
		t.Fatalf("RunCommand() execution = %+v, want nil", result.Execution)
	}
}
