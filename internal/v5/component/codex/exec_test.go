package codex

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/containerengine"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/go-clistate"
)

type fakeExecRuntime struct {
	workspace           string
	execErr             error
	lastMessage         string
	combinedOutputCalls int
	lastName            string
	lastArgs            []string
	stdoutJSON          string
}

func (r *fakeExecRuntime) Workspace() string { return r.workspace }
func (r *fakeExecRuntime) Exec(ctx context.Context, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	_, _ = ctx, stderr
	r.lastName = name
	r.lastArgs = append([]string(nil), args...)
	if r.stdoutJSON != "" && stdout != nil {
		_, _ = io.WriteString(stdout, r.stdoutJSON)
	}
	return r.execErr
}
func (r *fakeExecRuntime) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	_, _, _ = ctx, name, args
	r.combinedOutputCalls++
	return []byte(r.lastMessage), nil
}

func TestBuildExecArgsIncludesModelEffortAndResume(t *testing.T) {
	got := BuildExecArgs(ExecArgs{
		Workspace:        "/workspace",
		OutputPath:       "/tmp/out.txt",
		ProviderThreadID: "thread-1",
		Prompt:           "hello",
		DefaultModel:     "global-model",
		Options: TurnOptions{
			Model:           "thread-model",
			ReasoningEffort: "high",
		},
	})
	want := []string{
		"sh", "-lc", "rm -f " + containerengine.ActivePIDFile + "; echo $$ > " + containerengine.ActivePIDFile + "; exec \"$@\"", "sh",
		"codex", "-a", "never", "-s", "workspace-write", "exec", "--json", "--skip-git-repo-check",
		"--add-dir", "/workspace", "--output-last-message", "/tmp/out.txt", "-C", "/workspace",
		"-m", "thread-model", "-c", `model_reasoning_effort="high"`, "resume", "thread-1", "hello",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildExecArgs() = %#v, want %#v", got, want)
	}
}

func TestRunnerRunTurnStreamsAndReturnsLastMessage(t *testing.T) {
	withTempCwd(t, func(root string) {
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("new store: %v", err)
		}
		cfg, err := appstate.NewConfig(root, store)
		if err != nil {
			t.Fatalf("new config: %v", err)
		}
		runtime := &fakeExecRuntime{
			workspace:   "/workspace",
			lastMessage: "final reply",
			stdoutJSON: strings.Join([]string{
				`{"type":"thread.started","thread_id":"thread-123"}`,
				`{"type":"item.completed","item":{"type":"agent_message","text":"streamed reply"}}`,
			}, "\n") + "\n",
		}
		output := &capturingOutput{}
		result, err := NewRunner(cfg, nil).RunTurn(context.Background(), runtime, output, TurnRequest{Prompt: "hello"})
		if err != nil {
			t.Fatalf("RunTurn() error = %v", err)
		}
		if result.Reply != "final reply" || result.ProviderThreadID != "thread-123" {
			t.Fatalf("result = %#v", result)
		}
		if len(output.messages) != 1 || output.messages[0] != "streamed reply" {
			t.Fatalf("streamed messages = %#v", output.messages)
		}
		if runtime.combinedOutputCalls != 1 {
			t.Fatalf("combined output calls = %d", runtime.combinedOutputCalls)
		}
	})
}

func TestRunnerRunTurnReturnsCleanCancellationForRuntimeInterrupt(t *testing.T) {
	withTempCwd(t, func(root string) {
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("new store: %v", err)
		}
		cfg, err := appstate.NewConfig(root, store)
		if err != nil {
			t.Fatalf("new config: %v", err)
		}
		runtime := &fakeExecRuntime{
			workspace:  "/workspace",
			execErr:    context.Canceled,
			stdoutJSON: `{"type":"thread.started","thread_id":"thread-123"}` + "\n",
		}
		result, err := NewRunner(cfg, nil).RunTurn(context.Background(), runtime, nil, TurnRequest{Prompt: "hello"})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RunTurn() error = %v, want context.Canceled", err)
		}
		if result.ProviderThreadID != "thread-123" {
			t.Fatalf("provider thread id = %q", result.ProviderThreadID)
		}
		if runtime.combinedOutputCalls != 0 {
			t.Fatalf("combined output calls = %d, want 0", runtime.combinedOutputCalls)
		}
	})
}

type capturingOutput struct {
	messages []string
}

func (o *capturingOutput) Send(ctx context.Context, payload messenger.OutboundPayload) error {
	_ = ctx
	o.messages = append(o.messages, payload.Text.Text)
	return nil
}
