package codex

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestEventWriterCopiesAndExtractsEvents(t *testing.T) {
	var dst bytes.Buffer
	var logs []string
	var messages []string
	writer := newEventWriter(&dst, func(format string, args ...any) {
		logs = append(logs, strings.TrimSpace(fmt.Sprintf(format, args...)))
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
	for _, want := range []string{
		`codex json {"type":"thread.started","thread_id":"thread-123"}`,
		"codex json thread started provider_thread_id=thread-123",
		"codex json turn completed input_tokens=12 cached_input_tokens=3 output_tokens=4",
	} {
		if !containsLog(logs, want) {
			t.Fatalf("missing log %q: %#v", want, logs)
		}
	}
}

func TestEventWriterHandlesPartialWritesAndFinalLine(t *testing.T) {
	writer := newEventWriter(nil, nil)
	if _, err := writer.Write([]byte(`{"type":"thread.started"`)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if writer.ThreadID() != "" {
		t.Fatalf("ThreadID() before newline = %q", writer.ThreadID())
	}
	if _, err := writer.Write([]byte(",\"thread_id\":\"abc\"}\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if _, err := writer.Write([]byte(`{"type":"thread.started","thread_id":"def"}`)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	writer.Flush()
	if writer.ThreadID() != "def" {
		t.Fatalf("ThreadID() = %q", writer.ThreadID())
	}
}

func TestEventWriterSuppressesCodexProtocolAgentMessages(t *testing.T) {
	var logs []string
	var messages []string
	writer := newEventWriter(nil, func(format string, args ...any) {
		logs = append(logs, strings.TrimSpace(fmt.Sprintf(format, args...)))
	}, func(text string) {
		messages = append(messages, text)
	})

	input := strings.Join([]string{
		`{"type":"item.completed","item":{"type":"agent_message","text":"<tool_call>functions.exec_command agext:json {}</tool_call>"}}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"hello"}}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"<tool_result>{}</tool_result>"}}`,
	}, "\n") + "\n"

	if _, err := writer.Write([]byte(input)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	writer.Flush()

	if len(messages) != 1 || messages[0] != "hello" {
		t.Fatalf("messages = %#v, want [hello]", messages)
	}
	if !containsLog(logs, "codex json suppressed protocol agent message") {
		t.Fatalf("missing suppression log: %#v", logs)
	}
}

func TestExtractThreadIDIgnoresInvalidLines(t *testing.T) {
	jsonl := strings.Join([]string{
		`not-json`,
		`{"type":"item.started"}`,
		`{"type":"thread.started","thread_id":"abc-123"}`,
	}, "\n")
	if got := extractThreadID(jsonl); got != "abc-123" {
		t.Fatalf("extractThreadID() = %q, want abc-123", got)
	}
}

func TestTrimErrorDetail(t *testing.T) {
	if got := trimErrorDetail("  problem  "); got != "problem" {
		t.Fatalf("trimErrorDetail() = %q", got)
	}
	long := strings.Repeat("x", errorDetailMax+10)
	got := trimErrorDetail(long)
	if len(got) > errorDetailMax+3 {
		t.Fatalf("trimmed detail too long: %d", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("trimmed detail should have ellipsis")
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
