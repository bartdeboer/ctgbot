package llamacppagent

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/containerengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/toolloop"
)

func TestBuildExecArgsWrapsToolloopWithPIDFile(t *testing.T) {
	t.Parallel()
	args := BuildExecArgs(ToolloopTurnRequest{
		Env:              []string{"TOOLLOOP_MODEL=qwen"},
		ResultRuntime:    "/home/result.json",
		EventsRuntime:    "/home/events.jsonl",
		ProviderThreadID: "conv-1",
		Prompt:           "hello",
	})
	wantPrefix := []string{
		"sh",
		"-lc",
		"rm -f " + containerengine.ActivePIDFile + "; echo $$ > " + containerengine.ActivePIDFile + "; exec \"$@\"",
		"sh",
		"env",
		"TOOLLOOP_MODEL=qwen",
		"toolloop",
		"--output",
		"/home/result.json",
		"--events",
		"/home/events.jsonl",
		"resume",
		"conv-1",
		"--",
		"hello",
	}
	if strings.Join(args, "\x00") != strings.Join(wantPrefix, "\x00") {
		t.Fatalf("args = %#v, want %#v", args, wantPrefix)
	}
}

func TestRunnerForwardsReasoningEventsWhileToolloopRuns(t *testing.T) {
	dir := t.TempDir()
	eventsPath := filepath.Join(dir, "events.jsonl")
	resultPath := filepath.Join(dir, "result.json")
	runtime := fakeExecRuntime{run: func(ctx context.Context, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
		writeToolloopEvent(t, eventsPath, toolloop.Event{
			Type: "model.response",
			Data: map[string]any{"reasoning_preview": "I will inspect the workspace.", "reasoning_content_chars": 29},
		})
		time.Sleep(50 * time.Millisecond)
		data, _ := json.Marshal(toolloop.Result{Status: "success", Text: "done", ConversationID: "conv-1", Iterations: 1})
		return os.WriteFile(resultPath, append(data, '\n'), 0o600)
	}}
	output := &fakeOutputHandler{}

	result, err := (&Runner{EventPoll: 5 * time.Millisecond}).RunTurn(context.Background(), runtime, output, ToolloopTurnRequest{
		Prompt:        "hello",
		ResultHost:    resultPath,
		ResultRuntime: "/runtime/result.json",
		EventsHost:    eventsPath,
		EventsRuntime: "/runtime/events.jsonl",
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.Reply != "done" || result.ProviderThreadID != "conv-1" {
		t.Fatalf("result = %#v", result)
	}
	if len(output.texts) != 1 || output.texts[0] != "I will inspect the workspace." {
		t.Fatalf("forwarded texts = %#v", output.texts)
	}
	if output.payloads[0].Role != coremodel.MessageRoleAgent || output.payloads[0].Kind != coremodel.MessageKindProgress {
		t.Fatalf("forwarded payload = %#v", output.payloads[0])
	}
}

func writeToolloopEvent(t *testing.T, path string, event toolloop.Event) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		t.Fatal(err)
	}
}

type fakeExecRuntime struct {
	run func(context.Context, io.Writer, io.Writer, string, ...string) error
}

func (f fakeExecRuntime) Exec(ctx context.Context, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	if f.run == nil {
		return nil
	}
	return f.run(ctx, stdout, stderr, name, args...)
}

type fakeOutputHandler struct {
	texts    []string
	payloads []message.OutboundPayload
}

func (f *fakeOutputHandler) Send(ctx context.Context, payload message.OutboundPayload) error {
	_ = ctx
	f.payloads = append(f.payloads, payload)
	if text := strings.TrimSpace(payload.Text.Text); text != "" {
		f.texts = append(f.texts, text)
	}
	return nil
}
