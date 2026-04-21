package server

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/chatcommands"
)

func TestRunCommandRunnerExecutesAllowedCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("default allowed commands are empty on windows")
	}
	runner := &RunCommandRunner{ResolveAllowed: StaticAllowedCommandResolver(nil), DefaultTimeoutSec: 5}
	result, err := runner.ExecuteRunCommand(context.Background(), chatcommands.Request{}, chatcommands.RunCommand{Command: "pwd"})
	if err != nil {
		t.Fatalf("ExecuteRunCommand() error = %v", err)
	}
	if strings.TrimSpace(result.Text) == "" {
		t.Fatal("expected command output")
	}
}

func TestRunCommandRunnerRejectsUnknownCommand(t *testing.T) {
	runner := &RunCommandRunner{ResolveAllowed: StaticAllowedCommandResolver(nil), DefaultTimeoutSec: 5}
	_, err := runner.ExecuteRunCommand(context.Background(), chatcommands.Request{}, chatcommands.RunCommand{Command: "definitely-not-allowed"})
	if err == nil {
		t.Fatal("expected error")
	}
}
