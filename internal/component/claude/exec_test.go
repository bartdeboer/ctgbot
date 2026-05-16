package claude

import (
	"reflect"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/containerengine"
)

func TestBuildExecArgsStartsClaudePrintTurn(t *testing.T) {
	got := BuildExecArgs(ExecArgs{
		ProviderThreadID: "session-123",
		Prompt:           "hello",
		Options: TurnOptions{
			Model:          "opus",
			PermissionMode: "bypassPermissions",
			SystemPrompt:   "be brief",
		},
	})
	want := []string{
		"sh", "-lc", "rm -f " + containerengine.ActivePIDFile + "; echo $$ > " + containerengine.ActivePIDFile + "; exec \"$@\"", "sh",
		"claude", "-p", "hello", "--output-format", "json",
		"--exclude-dynamic-system-prompt-sections",
		"--model", "opus", "--permission-mode", "bypassPermissions", "--append-system-prompt", "be brief", "--resume", "session-123",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildExecArgs() = %#v, want %#v", got, want)
	}
}

func TestParseClaudeOutputReadsResultAndSessionID(t *testing.T) {
	got, err := parseClaudeOutput(`{"type":"result","result":"hi","session_id":"abc"}`)
	if err != nil {
		t.Fatalf("parseClaudeOutput() error = %v", err)
	}
	if got.Reply != "hi" || got.ProviderThreadID != "abc" {
		t.Fatalf("parseClaudeOutput() = %#v", got)
	}
}

func TestParseClaudeOutputFallsBackToMessageContent(t *testing.T) {
	got, err := parseClaudeOutput(`{"message":{"content":[{"text":"one"},{"text":"two"}]},"session_id":"abc"}`)
	if err != nil {
		t.Fatalf("parseClaudeOutput() error = %v", err)
	}
	if got.Reply != "one\n\ntwo" || got.ProviderThreadID != "abc" {
		t.Fatalf("parseClaudeOutput() = %#v", got)
	}
}
