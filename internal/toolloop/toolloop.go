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
	"strconv"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/toolloop/applypatch"
)

type Request struct {
	BaseURL       string    `json:"base_url"`
	APIKey        string    `json:"api_key,omitempty"`
	Model         string    `json:"model"`
	System        string    `json:"system,omitempty"`
	Messages      []Message `json:"messages,omitempty"`
	Prompt        string    `json:"prompt"`
	Workspace     string    `json:"workspace,omitempty"`
	MaxIterations int       `json:"max_iterations,omitempty"`
	MaxTokens     int       `json:"max_tokens,omitempty"`
	Temperature   float64   `json:"temperature,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Result struct {
	Text       string `json:"text,omitempty"`
	Iterations int    `json:"iterations"`
}

type Runner struct {
	Client         *http.Client
	HostbridgePath string
	Workspace      string
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
	if req.Prompt == "" && len(req.Messages) == 0 {
		return Result{}, errors.New("missing prompt")
	}
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	messages := []chatMessage{}
	if req.System != "" {
		messages = append(messages, chatMessage{Role: "system", Content: req.System})
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
	for i := 0; i < req.MaxIterations; i++ {
		resp, err := r.chat(ctx, client, req, messages)
		if err != nil {
			return Result{}, err
		}
		if len(resp.ToolCalls) == 0 {
			return Result{Text: strings.TrimSpace(resp.Content), Iterations: i + 1}, nil
		}
		messages = append(messages, chatMessage{Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls})
		for _, call := range resp.ToolCalls {
			toolText, isErr := r.executeTool(ctx, call)
			messages = append(messages, chatMessage{Role: "tool", ToolCallID: call.ID, Name: call.Function.Name, Content: toolText})
			if isErr && r.Stderr != nil {
				fmt.Fprintf(r.Stderr, "tool %s failed: %s\n", call.Function.Name, strings.TrimSpace(toolText))
			}
		}
	}
	return Result{}, fmt.Errorf("tool loop exceeded max iterations (%d)", req.MaxIterations)
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

func (r Runner) chat(ctx context.Context, client *http.Client, req Request, messages []chatMessage) (assistantMessage, error) {
	body := chatRequest{Model: req.Model, Messages: messages, Tools: hostbridgeTools(), MaxTokens: req.MaxTokens, Temperature: req.Temperature}
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
	return decoded.Choices[0].Message, nil
}

func (r Runner) executeTool(ctx context.Context, call toolCall) (string, bool) {
	name := strings.TrimSpace(call.Function.Name)
	switch name {
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
	default:
		return "unknown tool: " + name, true
	}
}

type hostbridgeArgs struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

type applyPatchArgs struct {
	Patch string `json:"patch"`
}

func (r Runner) applyPatch(ctx context.Context, args applyPatchArgs) (string, bool) {
	workspace := firstNonEmpty(r.Workspace, getenv("TOOLLOOP_WORKSPACE"), "/workspace")
	result, err := applypatch.Apply(ctx, applypatch.Request{Workspace: workspace, Patch: args.Patch})
	if err != nil {
		return err.Error(), true
	}
	return result.Summary, false
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

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Tools       []toolDef     `json:"tools,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type assistantMessage struct {
	Content   string     `json:"content"`
	ToolCalls []toolCall `json:"tool_calls"`
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
		Message assistantMessage `json:"message"`
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

func hostbridgeTools() []toolDef {
	return []toolDef{
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
				Name:        "apply_patch",
				Description: "Apply a Codex-style patch to files in the workspace. The patch must begin with *** Begin Patch and end with *** End Patch.",
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
		BaseURL:       getenv("TOOLLOOP_BASE_URL"),
		APIKey:        getenv("TOOLLOOP_API_KEY"),
		Model:         getenv("TOOLLOOP_MODEL"),
		System:        getenv("TOOLLOOP_SYSTEM"),
		Prompt:        prompt,
		Workspace:     getenv("TOOLLOOP_WORKSPACE"),
		MaxIterations: maxIterations,
		MaxTokens:     maxTokens,
		Temperature:   temperature,
	}
}

var getenv = os.Getenv
