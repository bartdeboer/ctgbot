package toolloop

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerExecutesHostbridgeToolAndReturnsFinal(t *testing.T) {
	t.Parallel()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{"id": "call_1", "type": "function", "function": map[string]any{"name": "hostbridge", "arguments": `{"command":"status","args":["--short"]}`}}}}}}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "done"}}}})
		}
	}))
	defer server.Close()

	result, err := (Runner{HostbridgePath: "/bin/echo"}).Run(context.Background(), Request{BaseURL: server.URL, Model: "qwen", Prompt: "hi", MaxIterations: 4})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Text != "done" || result.Iterations != 2 {
		t.Fatalf("result = %#v", result)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestRunnerExecutesApplyPatchTool(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	patch := `*** Begin Patch
*** Add File: hello.txt
+hello
*** End Patch`
	args, _ := json.Marshal(applyPatchArgs{Patch: patch})
	text, isErr := (Runner{Workspace: workspace}).executeTool(context.Background(), toolCall{Function: struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{Name: "apply_patch", Arguments: string(args)}})
	if isErr {
		t.Fatalf("apply_patch failed: %s", text)
	}
	if !strings.Contains(text, "add hello.txt") {
		t.Fatalf("tool output = %q", text)
	}
}

func TestRunnerExecutesShellToolInWorkspace(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(shellArgs{Command: "sed -n '1,1p' hello.txt"})
	text, isErr := (Runner{Workspace: workspace}).executeTool(context.Background(), toolCall{Function: struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{Name: "shell", Arguments: string(args)}})
	if isErr {
		t.Fatalf("shell failed: %s", text)
	}
	if text != "hello" {
		t.Fatalf("shell output = %q", text)
	}
}

func TestRunnerRejectsShellWorkdirOutsideWorkspace(t *testing.T) {
	t.Parallel()
	args, _ := json.Marshal(shellArgs{Command: "pwd", Workdir: "/tmp"})
	text, isErr := (Runner{Workspace: "/workspace"}).executeTool(context.Background(), toolCall{Function: struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{Name: "shell", Arguments: string(args)}})
	if !isErr || !strings.Contains(text, "outside workspace") {
		t.Fatalf("text=%q isErr=%t, want outside workspace error", text, isErr)
	}
}
