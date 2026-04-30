package codex

import (
	"reflect"
	"testing"
)

func TestParseCodexJSONOutput(t *testing.T) {
	output := `{"type":"thread.started","thread_id":"thread-1"}
{"type":"turn.started"}
{"type":"item.completed","item":{"type":"agent_message","text":"first"}}
{"type":"item.completed","item":{"type":"agent_message","text":"final reply"}}
{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":2}}`

	result, err := parseCodexJSONOutput(output)
	if err != nil {
		t.Fatalf("parseCodexJSONOutput() error = %v", err)
	}
	if result.Conversation.ThreadID != "thread-1" {
		t.Fatalf("ThreadID = %q, want thread-1", result.Conversation.ThreadID)
	}
	if result.Reply != "final reply" {
		t.Fatalf("Reply = %q, want final reply", result.Reply)
	}
}

func TestThreadConversationEncodingAndResumeID(t *testing.T) {
	conversation := CodexThreadConversation{ThreadID: "thread-1", SessionID: "session-1"}
	body, err := encodeThreadConversation(conversation)
	if err != nil {
		t.Fatalf("encodeThreadConversation() error = %v", err)
	}
	got, err := decodeThreadConversation(body)
	if err != nil {
		t.Fatalf("decodeThreadConversation() error = %v", err)
	}
	if !reflect.DeepEqual(got, conversation) {
		t.Fatalf("decoded conversation = %#v, want %#v", got, conversation)
	}
	if got.ResumeID() != "session-1" {
		t.Fatalf("ResumeID() = %q, want session-1", got.ResumeID())
	}
	if (CodexThreadConversation{ThreadID: "thread-1"}).ResumeID() != "thread-1" {
		t.Fatal("ResumeID() did not fall back to ThreadID")
	}
}

func TestCodexExecArgsResumesConversation(t *testing.T) {
	args := codexExecArgs("hello", CodexThreadConversation{ThreadID: "thread-1"})
	wantTail := []string{"resume", "thread-1", "hello"}
	if !reflect.DeepEqual(args[len(args)-3:], wantTail) {
		t.Fatalf("args tail = %#v, want %#v", args[len(args)-3:], wantTail)
	}

	args = codexExecArgs("hello", CodexThreadConversation{})
	if args[len(args)-1] != "hello" {
		t.Fatalf("args tail = %#v, want prompt only", args)
	}
}
