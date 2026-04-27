package codexengine

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestCodexJSONWriterCopiesAndExtractsEvents(t *testing.T) {
	var dst bytes.Buffer
	var logs []string
	var messages []string
	writer := newCodexJSONWriter(&dst, func(format string, args ...any) {
		logs = append(logs, sprintf(format, args...))
	}, func(text string) {
		messages = append(messages, text)
	})

	input := strings.Join([]string{
		`{"type":"thread.started","thread_id":"thread-123"}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"hello"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":12,"cached_input_tokens":3,"output_tokens":4}}`,
	}, "\n") + "\n"

	if _, err := writer.Write([]byte(input)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	writer.Flush()

	if dst.String() != input {
		t.Fatalf("copied output = %q, want %q", dst.String(), input)
	}
	if writer.ThreadID() != "thread-123" {
		t.Fatalf("ThreadID() = %q", writer.ThreadID())
	}
	if writer.AgentMessage() != "hello" {
		t.Fatalf("AgentMessage() = %q", writer.AgentMessage())
	}
	if len(messages) != 1 || messages[0] != "hello" {
		t.Fatalf("messages = %#v, want [hello]", messages)
	}
	if writer.InputTokens() != 12 || writer.CachedInputTokens() != 3 || writer.OutputTokens() != 4 {
		t.Fatalf("usage = %d/%d/%d", writer.InputTokens(), writer.CachedInputTokens(), writer.OutputTokens())
	}
	if !containsLog(logs, `codex json {"type":"thread.started","thread_id":"thread-123"}`) {
		t.Fatalf("missing raw json log: %#v", logs)
	}
	if !containsLog(logs, "codex json thread started provider_thread_id=thread-123") {
		t.Fatalf("missing thread log: %#v", logs)
	}
	if !containsLog(logs, "codex json turn completed input_tokens=12 cached_input_tokens=3 output_tokens=4") {
		t.Fatalf("missing usage log: %#v", logs)
	}
}

func TestCodexJSONWriterHandlesPartialWrites(t *testing.T) {
	var dst bytes.Buffer
	writer := newCodexJSONWriter(&dst, nil)

	if _, err := writer.Write([]byte(`{"type":"thread.started"`)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if writer.ThreadID() != "" {
		t.Fatalf("ThreadID() before newline = %q", writer.ThreadID())
	}
	if _, err := writer.Write([]byte(",\"thread_id\":\"abc\"}\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	writer.Flush()

	if writer.ThreadID() != "abc" {
		t.Fatalf("ThreadID() = %q", writer.ThreadID())
	}
}

func TestCodexJSONWriterFlushesFinalLine(t *testing.T) {
	writer := newCodexJSONWriter(nil, nil)
	if _, err := writer.Write([]byte(`{"type":"thread.started","thread_id":"abc"}`)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	writer.Flush()
	if writer.ThreadID() != "abc" {
		t.Fatalf("ThreadID() = %q", writer.ThreadID())
	}
}

func containsLog(logs []string, want string) bool {
	for _, log := range logs {
		if log == want {
			return true
		}
	}
	return false
}

func sprintf(format string, args ...any) string {
	return strings.TrimSpace(fmt.Sprintf(format, args...))
}
