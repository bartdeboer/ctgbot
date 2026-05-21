package toolloop

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
