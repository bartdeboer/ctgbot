package toolloop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
)

const defaultTimeout = 2 * time.Minute

type Request struct {
	BaseURL                 string    `json:"base_url"`
	APIKey                  string    `json:"api_key,omitempty"`
	Model                   string    `json:"model"`
	System                  string    `json:"system,omitempty"`
	Messages                []Message `json:"messages,omitempty"`
	Prompt                  string    `json:"prompt"`
	Workspace               string    `json:"workspace,omitempty"`
	MaxIterations           int       `json:"max_iterations,omitempty"`
	MaxTokens               int       `json:"max_tokens,omitempty"`
	Temperature             float64   `json:"temperature,omitempty"`
	ModelPromptInstructions string    `json:"model_prompt_instructions,omitempty"`
	ModelToolInstructions   string    `json:"model_tool_instructions,omitempty"`
	ModelReasoningFormat    string    `json:"model_reasoning_format,omitempty"`
	ModelToolCallFormat     string    `json:"model_tool_call_format,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Result struct {
	ConversationID string      `json:"conversation_id,omitempty"`
	Status         string      `json:"status,omitempty"`
	Text           string      `json:"text,omitempty"`
	Error          string      `json:"error,omitempty"`
	Iterations     int         `json:"iterations"`
	Trace          []TraceStep `json:"trace,omitempty"`
}

type TraceStep struct {
	Iteration             int               `json:"iteration"`
	FinishReason          string            `json:"finish_reason,omitempty"`
	AssistantContentChars int               `json:"assistant_content_chars"`
	AssistantPreview      string            `json:"assistant_preview,omitempty"`
	ToolCalls             []string          `json:"tool_calls,omitempty"`
	ToolResults           []TraceToolResult `json:"tool_results,omitempty"`
}

type TraceToolResult struct {
	Name          string `json:"name"`
	IsError       bool   `json:"is_error,omitempty"`
	OutputPreview string `json:"output_preview,omitempty"`
}

type Runner struct {
	Client         *http.Client
	HostbridgePath string
	ApplyPatchPath string
	ToolsPath      string
	Workspace      string
	Events         EventSink
	Stderr         io.Writer
	CommandTimeout time.Duration
}

func (r Runner) Run(ctx context.Context, req Request) (Result, error) {
	req = cleanRequest(req)
	if req.BaseURL == "" {
		return Result{}, errors.New("missing base_url")
	}
	if req.Model == "" {
		return Result{}, errors.New("missing model")
	}
	req.ModelPromptInstructions = strings.TrimSpace(req.ModelPromptInstructions)
	req.ModelToolInstructions = strings.TrimSpace(req.ModelToolInstructions)
	req.ModelReasoningFormat = strings.TrimSpace(req.ModelReasoningFormat)
	req.ModelToolCallFormat = strings.TrimSpace(req.ModelToolCallFormat)
	if req.Prompt == "" && len(req.Messages) == 0 {
		return Result{}, errors.New("missing prompt")
	}
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}
	messages := []chatMessage{}
	if system := systemPrompt(req); system != "" {
		messages = append(messages, chatMessage{Role: "system", Content: system})
	}
	if len(req.Messages) > 0 {
		for _, message := range req.Messages {
			role := strings.TrimSpace(message.Role)
			content := strings.TrimSpace(message.Content)
			if role == "" || content == "" {
				continue
			}
			messages = append(messages, chatMessage{Role: role, Content: content})
		}
	} else {
		messages = append(messages, chatMessage{Role: "user", Content: req.Prompt})
	}
	trace := []TraceStep{}
	r.emit(Event{Type: "turn.started", Data: map[string]any{"model": req.Model}})
	for i := 0; i < req.MaxIterations; i++ {
		iteration := i + 1
		r.emit(Event{Type: "model.request", Iteration: iteration, Data: map[string]any{"messages": len(messages)}})
		resp, err := r.chat(ctx, client, req, messages)
		if err != nil {
			result := errorResult(trace, i, err)
			r.emit(Event{Type: "turn.failed", Iteration: iteration, Error: err.Error()})
			return result, err
		}
		step := TraceStep{
			Iteration:             iteration,
			FinishReason:          strings.TrimSpace(resp.FinishReason),
			AssistantContentChars: len([]rune(resp.Content)),
			AssistantPreview:      previewText(resp.Content, 500),
			ToolCalls:             toolCallNames(resp.ToolCalls),
		}
		r.emit(Event{Type: "model.response", Iteration: iteration, Data: map[string]any{
			"finish_reason":           step.FinishReason,
			"assistant_content_chars": step.AssistantContentChars,
			"tool_calls":              step.ToolCalls,
		}})
		if len(resp.ToolCalls) == 0 {
			trace = append(trace, step)
			result := Result{Status: "success", Text: strings.TrimSpace(resp.Content), Iterations: iteration, Trace: trace}
			r.emit(Event{Type: "turn.completed", Iteration: iteration, Data: map[string]any{"text_chars": len([]rune(result.Text))}})
			return result, nil
		}
		messages = append(messages, chatMessage{Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls})
		for _, call := range resp.ToolCalls {
			r.emit(Event{Type: "tool.started", Iteration: iteration, ToolCall: call.ID, ToolName: call.Function.Name})
			toolText, isErr := r.executeTool(ctx, call)
			if strings.TrimSpace(toolText) == "" {
				toolText = "(no output)"
			}
			step.ToolResults = append(step.ToolResults, TraceToolResult{Name: call.Function.Name, IsError: isErr, OutputPreview: previewText(toolText, 500)})
			r.emit(Event{Type: "tool.finished", Iteration: iteration, ToolCall: call.ID, ToolName: call.Function.Name, Data: map[string]any{"is_error": isErr, "output_preview": previewText(toolText, 500)}})
			messages = append(messages, chatMessage{Role: "tool", ToolCallID: call.ID, Name: call.Function.Name, Content: toolText})
			if isErr && r.Stderr != nil {
				fmt.Fprintf(r.Stderr, "tool %s failed: %s\n", call.Function.Name, strings.TrimSpace(toolText))
			}
		}
		trace = append(trace, step)
	}
	err := fmt.Errorf("tool loop exceeded max iterations (%d)", req.MaxIterations)
	result := errorResult(trace, req.MaxIterations, err)
	r.emit(Event{Type: "turn.failed", Iteration: req.MaxIterations, Error: err.Error()})
	return result, err
}

func errorResult(trace []TraceStep, iterations int, err error) Result {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return Result{Status: "error", Error: message, Iterations: iterations, Trace: trace}
}

func cleanRequest(req Request) Request {
	req.BaseURL = strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	req.APIKey = strings.TrimSpace(req.APIKey)
	req.Model = strings.TrimSpace(req.Model)
	req.System = strings.TrimSpace(req.System)
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.MaxIterations <= 0 {
		req.MaxIterations = 20
	}
	if req.MaxIterations > 100 {
		req.MaxIterations = 100
	}
	return req
}

func systemPrompt(req Request) string {
	parts := []string{}
	if strings.TrimSpace(req.System) != "" {
		parts = append(parts, strings.TrimSpace(req.System))
	}
	if strings.TrimSpace(req.ModelPromptInstructions) != "" {
		parts = append(parts, strings.TrimSpace(req.ModelPromptInstructions))
	}
	if strings.TrimSpace(req.ModelToolInstructions) != "" {
		parts = append(parts, strings.TrimSpace(req.ModelToolInstructions))
	}
	metadata := []string{}
	if strings.TrimSpace(req.ModelReasoningFormat) != "" {
		metadata = append(metadata, "Reasoning format: "+strings.TrimSpace(req.ModelReasoningFormat))
	}
	if strings.TrimSpace(req.ModelToolCallFormat) != "" {
		metadata = append(metadata, "Tool call format: "+strings.TrimSpace(req.ModelToolCallFormat))
	}
	if len(metadata) > 0 {
		parts = append(parts, strings.Join(metadata, "\n"))
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func (r Runner) chat(ctx context.Context, client *http.Client, req Request, messages []chatMessage) (assistantMessage, error) {
	body := chatRequest{Model: req.Model, Messages: messages, Tools: toolDefinitions(), MaxTokens: req.MaxTokens, Temperature: req.Temperature}
	data, err := json.Marshal(body)
	if err != nil {
		return assistantMessage{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.BaseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return assistantMessage{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if req.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return assistantMessage{}, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return assistantMessage{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return assistantMessage{}, fmt.Errorf("chat completion status %s: %s", resp.Status, strings.TrimSpace(string(payload)))
	}
	var decoded chatResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return assistantMessage{}, fmt.Errorf("decode chat completion: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return assistantMessage{}, nil
	}
	message := decoded.Choices[0].Message
	message.FinishReason = decoded.Choices[0].FinishReason
	return message, nil
}

func toolCallNames(calls []toolCall) []string {
	if len(calls) == 0 {
		return nil
	}
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, strings.TrimSpace(call.Function.Name))
	}
	return names
}

func previewText(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if text == "" || maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "\n...<truncated>..."
}

func (r Runner) executeTool(ctx context.Context, call toolCall) (string, bool) {
	name := strings.TrimSpace(call.Function.Name)
	switch name {
	case "shell":
		var args shellArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "invalid shell arguments: " + err.Error(), true
		}
		return r.runShell(ctx, args)
	case "hostbridge":
		var args hostbridgeArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "invalid hostbridge arguments: " + err.Error(), true
		}
		return r.runHostbridge(ctx, args)
	case "apply_patch":
		var args applyPatchArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "invalid apply_patch arguments: " + err.Error(), true
		}
		return r.applyPatch(ctx, args)
	case "read":
		var args readFileArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "invalid read arguments: " + err.Error(), true
		}
		return r.readFile(ctx, args)
	case "write":
		var args writeFileArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "invalid write arguments: " + err.Error(), true
		}
		return r.writeFile(ctx, args)
	case "edit":
		var args editFileArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "invalid edit arguments: " + err.Error(), true
		}
		return r.editFile(ctx, args)
	default:
		return "unknown tool: " + name, true
	}
}

type hostbridgeArgs struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

type shellArgs struct {
	Command   string `json:"command"`
	Workdir   string `json:"workdir,omitempty"`
	TimeoutMS int    `json:"timeout_ms,omitempty"`
}

type readFileArgs struct {
	File   string `json:"file"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Pages  string `json:"pages,omitempty"`
}

type writeFileArgs struct {
	File    string `json:"file"`
	Content string `json:"content"`
}

type editFileArgs struct {
	File       string `json:"file"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

func (r Runner) runShell(ctx context.Context, args shellArgs) (string, bool) {
	command := strings.TrimSpace(args.Command)
	if command == "" {
		return "missing shell command", true
	}
	workspace := firstNonEmpty(r.Workspace, getenv("TOOLLOOP_WORKSPACE"), "/workspace")
	workdir, err := resolveWorkdir(workspace, args.Workdir)
	if err != nil {
		return err.Error(), true
	}
	timeout := r.CommandTimeout
	if args.TimeoutMS > 0 {
		timeout = time.Duration(args.TimeoutMS) * time.Millisecond
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	toolCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(toolCtx, "/bin/bash", "-c", command)
	cmd.Dir = workdir
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			text = err.Error()
		} else {
			text = err.Error() + "\n" + text
		}
		return text, true
	}
	return text, false
}

func (r Runner) readFile(ctx context.Context, args readFileArgs) (string, bool) {
	file, err := r.resolveWorkspaceFile(args.File)
	if err != nil {
		return err.Error(), true
	}
	argv := []string{"read", "--file", file}
	if args.Offset > 0 {
		argv = append(argv, "--offset", strconv.Itoa(args.Offset))
	}
	if args.Limit > 0 {
		argv = append(argv, "--limit", strconv.Itoa(args.Limit))
	}
	if strings.TrimSpace(args.Pages) != "" {
		argv = append(argv, "--pages", strings.TrimSpace(args.Pages))
	}
	return r.runTools(ctx, argv, "")
}

func (r Runner) writeFile(ctx context.Context, args writeFileArgs) (string, bool) {
	file, err := r.resolveWorkspaceFile(args.File)
	if err != nil {
		return err.Error(), true
	}
	return r.runTools(ctx, []string{"write", "--file", file}, args.Content)
}

func (r Runner) editFile(ctx context.Context, args editFileArgs) (string, bool) {
	file, err := r.resolveWorkspaceFile(args.File)
	if err != nil {
		return err.Error(), true
	}
	argv := []string{"edit", "--file", file, "--old", args.OldString, "--new", args.NewString}
	if args.ReplaceAll {
		argv = append(argv, "--replace-all")
	}
	return r.runTools(ctx, argv, "")
}

func (r Runner) runTools(ctx context.Context, argv []string, stdin string) (string, bool) {
	timeout := r.CommandTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	toolCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	binary := firstNonEmpty(r.ToolsPath, getenv("TOOLLOOP_TOOLS_PATH"), "tools")
	cmd := exec.CommandContext(toolCtx, binary, argv...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			text = err.Error()
		} else {
			text = err.Error() + "\n" + text
		}
		return text, true
	}
	return text, false
}

func (r Runner) resolveWorkspaceFile(requested string) (string, error) {
	workspace := firstNonEmpty(r.Workspace, getenv("TOOLLOOP_WORKSPACE"), "/workspace")
	workspace = path.Clean(workspace)
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", errors.New("missing file")
	}
	var file string
	if strings.HasPrefix(requested, "/") {
		file = path.Clean(requested)
	} else {
		cleaned := path.Clean(requested)
		if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			return "", fmt.Errorf("file escapes workspace: %s", requested)
		}
		file = path.Join(workspace, cleaned)
	}
	if file == workspace || !strings.HasPrefix(file, strings.TrimRight(workspace, "/")+"/") {
		return "", fmt.Errorf("file outside workspace: %s", requested)
	}
	return file, nil
}

type applyPatchArgs struct {
	Patch string `json:"patch"`
}

func (r Runner) applyPatch(ctx context.Context, args applyPatchArgs) (string, bool) {
	patch := strings.TrimSpace(args.Patch)
	if patch == "" {
		return "missing patch", true
	}
	workspace := firstNonEmpty(r.Workspace, getenv("TOOLLOOP_WORKSPACE"), "/workspace")
	timeout := r.CommandTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	toolCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	binary := firstNonEmpty(r.ApplyPatchPath, getenv("TOOLLOOP_APPLY_PATCH_PATH"), "apply_patch")
	cmd := exec.CommandContext(toolCtx, binary)
	cmd.Dir = workspace
	cmd.Stdin = strings.NewReader(args.Patch)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			text = err.Error()
		} else {
			text = err.Error() + "\n" + text
		}
		return text, true
	}
	return text, false
}

func (r Runner) runHostbridge(ctx context.Context, args hostbridgeArgs) (string, bool) {
	command := strings.TrimSpace(args.Command)
	if command == "" {
		return "missing hostbridge command", true
	}
	timeout := r.CommandTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	toolCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	binary := strings.TrimSpace(r.HostbridgePath)
	if binary == "" {
		binary = "hostbridge"
	}
	argv := append([]string{command}, cleanArgs(args.Args)...)
	cmd := exec.CommandContext(toolCtx, binary, argv...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			text = err.Error()
		} else {
			text = err.Error() + "\n" + text
		}
		return text, true
	}
	return text, false
}

func cleanArgs(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strings.TrimSpace(value))
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func resolveWorkdir(workspace string, requested string) (string, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = "/workspace"
	}
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return workspace, nil
	}
	if strings.HasPrefix(requested, "/") {
		if requested == workspace || strings.HasPrefix(requested, strings.TrimRight(workspace, "/")+"/") {
			return requested, nil
		}
		return "", fmt.Errorf("workdir outside workspace: %s", requested)
	}
	cleaned := path.Clean(requested)
	if cleaned == "." {
		return workspace, nil
	}
	if strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("workdir escapes workspace: %s", requested)
	}
	return strings.TrimRight(workspace, "/") + "/" + cleaned, nil
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Tools       []toolDef     `json:"tools,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type assistantMessage struct {
	Content      string     `json:"content"`
	ToolCalls    []toolCall `json:"tool_calls"`
	FinishReason string     `json:"-"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatResponse struct {
	Choices []struct {
		Message      assistantMessage `json:"message"`
		FinishReason string           `json:"finish_reason"`
	} `json:"choices"`
}

type toolDef struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

func toolDefinitions() []toolDef {
	return []toolDef{
		{
			Type: "function",
			Function: toolFunction{
				Name: "shell",
				Description: `Run a bash command inside the sandbox workspace.
Use shell for normal commands such as go test, go run, rg, find, ls, sed, and nl.
Prefer read-only inspection before editing. For file edits, prefer edit/write or apply_patch instead of shell redirection.`,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command":    map[string]any{"type": "string", "description": "Bash command to run, for example rg -n \"functionName\" path or nl -ba file | sed -n '120,180p'."},
						"workdir":    map[string]any{"type": "string", "description": "Workspace-relative directory. Defaults to /workspace."},
						"timeout_ms": map[string]any{"type": "integer", "description": "Optional timeout in milliseconds."},
					},
					"required": []string{"command"},
				},
			},
		},
		{
			Type: "function",
			Function: toolFunction{
				Name:        "hostbridge",
				Description: "Run one allowed hostbridge command inside the current ctgbot sandbox. Use only commands shown by hostbridge help.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]any{"type": "string", "description": "The hostbridge command name, for example status, ls, pwd, or semantic."},
						"args":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Command arguments."},
					},
					"required": []string{"command"},
				},
			},
		},
		{
			Type: "function",
			Function: toolFunction{
				Name: "read",
				Description: `Read a workspace file using Claude-style file-tool semantics.
Use this before edit or before overwriting an existing file with write.
The file may be absolute under /workspace or workspace-relative. Text output is line-numbered; use offset/limit for large files. PDFs require pages.`,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"file":   map[string]any{"type": "string", "description": "File path under the workspace, for example /workspace/main.go or main.go."},
						"offset": map[string]any{"type": "integer", "description": "Optional 0-based line offset."},
						"limit":  map[string]any{"type": "integer", "description": "Optional maximum number of lines to read."},
						"pages":  map[string]any{"type": "string", "description": "Required page range for PDF files, for example 1-3."},
					},
					"required": []string{"file"},
				},
			},
		},
		{
			Type: "function",
			Function: toolFunction{
				Name: "write",
				Description: `Create or fully rewrite a workspace file using Claude-style file-tool semantics.
Use write for new files or deliberate full-file rewrites. Existing files must be read first with read.
For localized changes to existing files, prefer edit. The file may be absolute under /workspace or workspace-relative.`,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"file":    map[string]any{"type": "string", "description": "File path under the workspace, for example /workspace/main.go or main.go."},
						"content": map[string]any{"type": "string", "description": "Full replacement file content."},
					},
					"required": []string{"file", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: toolFunction{
				Name: "edit",
				Description: `Edit an existing workspace file by exact string replacement using Claude-style file-tool semantics.
The file must be read first with read. old_string must exactly match existing text.
By default old_string must occur exactly once; if it appears multiple times, include more context or intentionally set replace_all=true.`,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"file":        map[string]any{"type": "string", "description": "File path under the workspace, for example /workspace/main.go or main.go."},
						"old_string":  map[string]any{"type": "string", "description": "Exact existing text to replace. Include enough context to make it unique."},
						"new_string":  map[string]any{"type": "string", "description": "Replacement text."},
						"replace_all": map[string]any{"type": "boolean", "description": "Replace every occurrence. Use only when all matches should change."},
					},
					"required": []string{"file", "old_string", "new_string"},
				},
			},
		},
		{
			Type: "function",
			Function: toolFunction{
				Name: "apply_patch",
				Description: `Apply a Codex-style patch to workspace files.
This is Codex apply_patch grammar, not unified diff. File paths in the patch must be relative, never absolute.
Patch envelope:
*** Begin Patch
... file operations ...
*** End Patch
Add file:
*** Add File: path
+line
+line
Delete file:
*** Delete File: path
Update file:
*** Update File: path
@@ optional context/header
 context line
-old line
+new line
Optional rename after update header:
*** Move to: new/path
Do not emit --- /dev/null, +++ b/file, or @@ -0,0 unified-diff headers.
Minimal valid example:
*** Begin Patch
*** Add File: hello.txt
+hello
*** End Patch`,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"patch": map[string]any{"type": "string", "description": "Codex-style patch text."},
					},
					"required": []string{"patch"},
				},
			},
		},
	}
}

func RequestFromEnv(prompt string) Request {
	maxIterations, _ := strconv.Atoi(strings.TrimSpace(getenv("TOOLLOOP_MAX_ITERATIONS")))
	maxTokens, _ := strconv.Atoi(strings.TrimSpace(getenv("TOOLLOOP_MAX_TOKENS")))
	temperature, _ := strconv.ParseFloat(strings.TrimSpace(getenv("TOOLLOOP_TEMPERATURE")), 64)
	return Request{
		BaseURL:                 getenv("TOOLLOOP_BASE_URL"),
		APIKey:                  getenv("TOOLLOOP_API_KEY"),
		Model:                   getenv("TOOLLOOP_MODEL"),
		System:                  getenv("TOOLLOOP_SYSTEM"),
		Prompt:                  prompt,
		Workspace:               getenv("TOOLLOOP_WORKSPACE"),
		MaxIterations:           maxIterations,
		MaxTokens:               maxTokens,
		Temperature:             temperature,
		ModelPromptInstructions: getenv("TOOLLOOP_MODEL_PROMPT_INSTRUCTIONS"),
		ModelToolInstructions:   getenv("TOOLLOOP_MODEL_TOOL_INSTRUCTIONS"),
		ModelReasoningFormat:    getenv("TOOLLOOP_MODEL_REASONING_FORMAT"),
		ModelToolCallFormat:     getenv("TOOLLOOP_MODEL_TOOL_CALL_FORMAT"),
	}
}

var getenv = os.Getenv
