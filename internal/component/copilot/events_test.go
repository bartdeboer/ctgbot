package copilot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const fixtureID = "11111111-1111-4111-8111-111111111111"
const otherID = "22222222-2222-4222-8222-222222222222"

func jsonLine(value any) string { body, _ := json.Marshal(value); return string(body) + "\n" }
func terminal(id string, code int) string {
	return jsonLine(map[string]any{"type": "result", "sessionId": id, "exitCode": code})
}
func fullMessage(text string) string {
	return jsonLine(map[string]any{"type": "assistant.message", "data": map[string]any{"messageId": "m", "content": text}})
}
func startEvent(id string) string {
	return jsonLine(map[string]any{"type": "session.start", "data": map[string]any{"sessionId": id}})
}

func TestOutputContract(t *testing.T) {
	good := startEvent(fixtureID) + fullMessage("Final reply") + terminal(fixtureID, 0)
	cases := []struct {
		name, output, reply, id string
		fail                    bool
	}{
		{"normal", good, "Final reply", fixtureID, false},
		{"no-newline", strings.TrimSuffix(good, "\n"), "Final reply", fixtureID, false},
		{"empty", "", "", "", true},
		{"malformed", good + "{broken", "", "", true},
		{"unknown-event", jsonLine(map[string]any{"type": "future.event", "data": map[string]any{"content": "not an answer"}}) + good, "Final reply", fixtureID, false},
		{"no-terminal", fullMessage("partial"), "", "", true},
		{"start-only", startEvent(fixtureID), "", "", true},
		{"empty-answer", terminal(fixtureID, 0), "", "", true},
		{"wrong-result-id", fullMessage("ok") + terminal(otherID, 0), "", "", true},
		{"wrong-start-id", startEvent(otherID) + fullMessage("ok") + terminal(fixtureID, 0), "", "", true},
		{"invalid-id", terminal("latest", 1), "", "", true},
		{"failed-terminal-preserves-validated-id", startEvent(fixtureID) + terminal(fixtureID, 1), "", fixtureID, true},
		{"failed-terminal-malformed-stream-does-not-bind", fullMessage("x") + "not-json\n" + terminal(fixtureID, 1), "", "", true},
		{"no-exit-code", `{"type":"result","sessionId":"` + fixtureID + `"}`, "", "", true},
		{"wrong-exit-code", terminal(fixtureID, 42), "", "", true},
		{"duplicate-terminal", good + terminal(fixtureID, 0), "", "", true},
		{"missing-content", `{"type":"assistant.message","data":{"messageId":"m"}}` + "\n" + terminal(fixtureID, 0), "", "", true},
		{"null", "null\n", "", "", true},
		{"retriable-error", `{"type":"session.error","data":{"errorType":"model_call","message":"retry"}}` + "\n" + good, "Final reply", fixtureID, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Deliberately split across JSON tokens and UTF-8 boundaries.
			parser := newEventWriter(fixtureID)
			for _, b := range []byte(tc.output) {
				if n, err := parser.Write([]byte{b}); n != 1 || err != nil {
					t.Fatal(n, err)
				}
			}
			result, err := parser.finish()
			if (err != nil) != tc.fail || result.Reply != tc.reply || result.ProviderThreadID != tc.id {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestOutputDoesNotRelayDeltasReasoningOrSubagents(t *testing.T) {
	parser := newEventWriter(fixtureID)
	stream := `{"type":"assistant.message_delta","data":{"messageId":"m","deltaContent":"duplicate"}}
{"type":"assistant.reasoning","data":{"content":"private reasoning"}}
{"type":"assistant.message","data":{"messageId":"tool","content":"working","toolRequests":[{}]}}
` + fullMessage("Final ✓") + `{"type":"assistant.message","agentId":"child","data":{"messageId":"sub","content":"subagent"}}
{"type":"assistant.message","data":{"messageId":"legacy","content":"legacy subagent","parentToolCallId":"tool"}}
` + terminal(fixtureID, 0)
	parser.Write([]byte(stream))
	result, err := parser.finish()
	if err != nil || result.Reply != "Final ✓" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestChunksAndLimits(t *testing.T) {
	chunk := func(index, count int, text string) string {
		return jsonLine(map[string]any{"type": "assistant.message", "data": map[string]any{"messageId": "chunk", "content": text, "chunkIndex": index, "chunkCount": count, "apiCallId": "call"}})
	}
	for _, tc := range []struct {
		name, body string
		fail       bool
	}{
		{"complete", chunk(0, 2, "Hello ") + chunk(1, 2, "world"), false},
		{"missing", chunk(0, 2, "Hello "), true},
		{"out-of-order", chunk(1, 2, "world"), true},
		{"new-tool-clears-answer", fullMessage("intermediate") + `{"type":"assistant.message","data":{"messageId":"tool","content":"","toolRequests":[{}]}}` + "\n", true},
		{"oversized-record", strings.Repeat("x", maxEventBytes+1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newEventWriter(fixtureID)
			p.Write([]byte(tc.body + terminal(fixtureID, 0)))
			r, err := p.finish()
			if (err != nil) != tc.fail {
				t.Fatalf("result=%+v err=%v", r, err)
			}
			if !tc.fail && r.Reply != "Hello world" {
				t.Fatal(r)
			}
		})
	}
	p := newEventWriter(fixtureID)
	record := jsonLine(map[string]any{"type": "ignored", "data": strings.Repeat("x", maxEventBytes/2)})
	for i := 0; i < 70; i++ {
		p.Write([]byte(record))
	}
	if _, err := p.finish(); err == nil || len(p.pending) > maxEventBytes {
		t.Fatalf("error=%v retained=%d", err, len(p.pending))
	}
}

func TestSessionUUIDs(t *testing.T) {
	a, b := newSessionID(), newSessionID()
	if a == b {
		t.Fatal("duplicate UUID")
	}
	for _, v := range []string{a, b, fixtureID, strings.ToUpper(a)} {
		if _, err := canonicalSessionID(v); err != nil {
			t.Fatal(v, err)
		}
	}
	for _, v := range []string{"latest", "prefix", "00000000-0000-0000-0000-000000000000", fixtureID + " ", "--resume=x"} {
		if _, err := canonicalSessionID(v); err == nil {
			t.Fatal(v)
		}
	}
}

func TestPinnedSyntheticFixture(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	body, err := os.ReadFile(filepath.Join(filepath.Dir(source), "testdata", "normal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	parser := newEventWriter(fixtureID)
	parser.Write(body)
	result, err := parser.finish()
	if err != nil || result.Reply != "Synthetic reply." || result.ProviderThreadID != fixtureID {
		t.Fatal(result, err)
	}
}
