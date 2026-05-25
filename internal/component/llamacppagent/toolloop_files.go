package llamacppagent

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/toolloop"
)

type toolloopInvocation struct {
	BaseURL                 string  `json:"base_url"`
	Model                   string  `json:"model"`
	Prompt                  string  `json:"prompt"`
	Workspace               string  `json:"workspace"`
	MaxIterations           int     `json:"max_iterations,omitempty"`
	MaxTokens               int     `json:"max_tokens,omitempty"`
	Temperature             float64 `json:"temperature,omitempty"`
	ModelPromptInstructions string  `json:"model_prompt_instructions,omitempty"`
	ModelToolInstructions   string  `json:"model_tool_instructions,omitempty"`
	ModelReasoningFormat    string  `json:"model_reasoning_format,omitempty"`
	ModelToolCallFormat     string  `json:"model_tool_call_format,omitempty"`
}

type toolloopRunFiles struct {
	HostDir        string
	RuntimeDir     string
	InvocationHost string
	ResultHost     string
	EventsHost     string
}

func newToolloopRunFiles(hostHome string, runtimeHome string, threadID modeluuid.UUID) (*toolloopRunFiles, error) {
	runName := threadID.String() + "-" + modeluuid.New().String()
	hostDir := filepath.Join(hostHome, "toolloop", "turns", runName)
	if err := os.MkdirAll(hostDir, 0o700); err != nil {
		return nil, err
	}
	runtimeDir := filepath.ToSlash(filepath.Join(runtimeHome, "toolloop", "turns", runName))
	return &toolloopRunFiles{
		HostDir:        hostDir,
		RuntimeDir:     runtimeDir,
		InvocationHost: filepath.Join(hostDir, "invocation.json"),
		ResultHost:     filepath.Join(hostDir, "result.json"),
		EventsHost:     filepath.Join(hostDir, "events.jsonl"),
	}, nil
}

func (f *toolloopRunFiles) Cleanup() {
	if f != nil {
		_ = os.RemoveAll(f.HostDir)
	}
}

func (f *toolloopRunFiles) WriteInvocation(invocation toolloopInvocation) error {
	data, err := json.MarshalIndent(invocation, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(f.InvocationHost, data, 0o600)
}

func (f *toolloopRunFiles) ResultRuntime() string {
	return filepath.ToSlash(filepath.Join(f.RuntimeDir, "result.json"))
}

func (f *toolloopRunFiles) EventsRuntime() string {
	return filepath.ToSlash(filepath.Join(f.RuntimeDir, "events.jsonl"))
}

func (f *toolloopRunFiles) DebugFiles() toolloop.DebugFiles {
	return toolloop.DebugFiles{Request: f.InvocationHost, Result: f.ResultHost, Events: f.EventsHost}
}
