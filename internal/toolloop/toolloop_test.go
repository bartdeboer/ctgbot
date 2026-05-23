package toolloop

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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
	workspace := t.TempDir()
	applyPatchPath := buildApplyPatchBinary(t)
	patch := `*** Begin Patch
*** Add File: hello.txt
+hello
*** End Patch`
	args, _ := json.Marshal(applyPatchArgs{Patch: patch})
	text, isErr := (Runner{Workspace: workspace, ApplyPatchPath: applyPatchPath}).executeTool(context.Background(), toolCall{Function: struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{Name: "apply_patch", Arguments: string(args)}})
	if isErr {
		t.Fatalf("apply_patch failed: %s", text)
	}
	if !strings.Contains(text, "A hello.txt") {
		t.Fatalf("tool output = %q", text)
	}
}

func TestRunnerExecutesReadWriteEditFileTools(t *testing.T) {
	workspace := t.TempDir()
	toolsPath := buildToolsBinary(t)

	runner := Runner{Workspace: workspace, ToolsPath: toolsPath}
	writeArgs, _ := json.Marshal(writeFileArgs{File: "hello.txt", Content: "hello\n"})
	text, isErr := runner.executeTool(context.Background(), toolCall{Function: struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{Name: "write_file", Arguments: string(writeArgs)}})
	if isErr {
		t.Fatalf("write_file failed: %s", text)
	}

	readArgs, _ := json.Marshal(readFileArgs{File: filepath.Join(workspace, "hello.txt"), Limit: 1})
	text, isErr = runner.executeTool(context.Background(), toolCall{Function: struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{Name: "read_file", Arguments: string(readArgs)}})
	if isErr {
		t.Fatalf("read_file failed: %s", text)
	}
	if !strings.Contains(text, "hello") {
		t.Fatalf("read_file output = %q", text)
	}

	editArgs, _ := json.Marshal(editFileArgs{File: "hello.txt", OldString: "hello", NewString: "world"})
	text, isErr = runner.executeTool(context.Background(), toolCall{Function: struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{Name: "edit_file", Arguments: string(editArgs)}})
	if isErr {
		t.Fatalf("edit_file failed: %s", text)
	}
	body, err := os.ReadFile(filepath.Join(workspace, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "world\n" {
		t.Fatalf("file body = %q", string(body))
	}
}

func TestRunnerRejectsFileToolOutsideWorkspace(t *testing.T) {
	t.Parallel()
	args, _ := json.Marshal(readFileArgs{File: "/tmp/nope.txt"})
	text, isErr := (Runner{Workspace: "/workspace", ToolsPath: "/bin/false"}).executeTool(context.Background(), toolCall{Function: struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{Name: "read_file", Arguments: string(args)}})
	if !isErr || !strings.Contains(text, "outside workspace") {
		t.Fatalf("text=%q isErr=%t, want outside workspace error", text, isErr)
	}
}

func buildApplyPatchBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "apply_patch")
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", bin, "./cmd/apply_patch")
	cmd.Dir = filepath.Join("..", "..")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build apply_patch: %v\n%s", err, strings.TrimSpace(string(out)))
	}
	return bin
}

func buildToolsBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "tools")
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", bin, "./cmd/tools")
	cmd.Dir = filepath.Join("..", "..")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build tools: %v\n%s", err, strings.TrimSpace(string(out)))
	}
	return bin
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

func TestChatMessageAlwaysIncludesContent(t *testing.T) {
	data, err := json.Marshal(chatMessage{Role: "tool", ToolCallID: "call_1", Name: "shell", Content: ""})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"content"`) {
		t.Fatalf("marshaled tool message should include content field: %s", data)
	}
}
