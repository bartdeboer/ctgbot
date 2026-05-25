package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/toolloop"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	return runArgs(os.Args[1:])
}

type cliOptions struct {
	requestPath     string
	outputPath      string
	eventsPath      string
	conversationDir string
	overrides       requestOverrides
}

type requestOverrides struct {
	baseURLSet                 bool
	baseURL                    string
	apiKeySet                  bool
	apiKey                     string
	modelSet                   bool
	model                      string
	systemSet                  bool
	system                     string
	workspaceSet               bool
	workspace                  string
	maxIterationsSet           bool
	maxIterations              int
	maxTokensSet               bool
	maxTokens                  int
	temperatureSet             bool
	temperature                float64
	modelPromptInstructionsSet bool
	modelPromptInstructions    string
	modelToolInstructionsSet   bool
	modelToolInstructions      string
	modelReasoningFormatSet    bool
	modelReasoningFormat       string
	modelToolCallFormatSet     bool
	modelToolCallFormat        string
}

func runArgs(args []string) error {
	opts, rest, err := parseCLI(args)
	if err != nil {
		return err
	}

	events, closeEvents, err := openEvents(opts.eventsPath)
	if err != nil {
		return err
	}
	defer closeEvents()

	if opts.requestPath != "" || len(rest) == 0 {
		return runRequestMode(opts.requestPath, opts.outputPath, events, opts.overrides)
	}
	return runConversationMode(rest, opts.conversationDir, opts.outputPath, events, opts.overrides)
}

func parseCLI(args []string) (cliOptions, []string, error) {
	var opts cliOptions
	var llamaBackend string
	var baseURL string
	fs := flag.NewFlagSet("toolloop", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&opts.requestPath, "request", "", "path to JSON toolloop request; stdin is used when empty")
	fs.StringVar(&opts.outputPath, "output", "", "path to write JSON result; stdout is used when empty")
	fs.StringVar(&opts.eventsPath, "events", "", "path to write JSONL event stream")
	fs.StringVar(&opts.conversationDir, "conversation-dir", "", "directory for toolloop conversation JSON files")
	fs.StringVar(&baseURL, "base-url", "", "OpenAI-compatible API base URL")
	fs.StringVar(&llamaBackend, "llama-backend", "", "alias for --base-url")
	fs.StringVar(&opts.overrides.apiKey, "api-key", "", "API key for the OpenAI-compatible backend")
	fs.StringVar(&opts.overrides.model, "model", "", "model name to request from the backend")
	fs.StringVar(&opts.overrides.system, "system", "", "system prompt")
	fs.StringVar(&opts.overrides.workspace, "workspace", "", "workspace path exposed to tools")
	fs.IntVar(&opts.overrides.maxIterations, "max-iterations", 0, "maximum tool-loop iterations")
	fs.IntVar(&opts.overrides.maxTokens, "max-tokens", 0, "maximum completion tokens")
	fs.Float64Var(&opts.overrides.temperature, "temperature", 0, "sampling temperature")
	fs.StringVar(&opts.overrides.modelPromptInstructions, "model-prompt-instructions", "", "model-specific prompt instructions")
	fs.StringVar(&opts.overrides.modelToolInstructions, "model-tool-instructions", "", "model-specific tool instructions")
	fs.StringVar(&opts.overrides.modelReasoningFormat, "model-reasoning-format", "", "model-specific reasoning format note")
	fs.StringVar(&opts.overrides.modelToolCallFormat, "model-tool-call-format", "", "model-specific tool-call format note")
	if err := fs.Parse(args); err != nil {
		return cliOptions{}, nil, err
	}

	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	if visited["base-url"] && visited["llama-backend"] && strings.TrimSpace(baseURL) != strings.TrimSpace(llamaBackend) {
		return cliOptions{}, nil, fmt.Errorf("--base-url and --llama-backend disagree")
	}
	if visited["base-url"] || visited["llama-backend"] {
		opts.overrides.baseURLSet = true
		opts.overrides.baseURL = strings.TrimSpace(firstNonEmpty(baseURL, llamaBackend))
	}
	opts.overrides.apiKeySet = visited["api-key"]
	opts.overrides.modelSet = visited["model"]
	opts.overrides.systemSet = visited["system"]
	opts.overrides.workspaceSet = visited["workspace"]
	opts.overrides.maxIterationsSet = visited["max-iterations"]
	opts.overrides.maxTokensSet = visited["max-tokens"]
	opts.overrides.temperatureSet = visited["temperature"]
	opts.overrides.modelPromptInstructionsSet = visited["model-prompt-instructions"]
	opts.overrides.modelToolInstructionsSet = visited["model-tool-instructions"]
	opts.overrides.modelReasoningFormatSet = visited["model-reasoning-format"]
	opts.overrides.modelToolCallFormatSet = visited["model-tool-call-format"]
	opts.requestPath = strings.TrimSpace(opts.requestPath)
	opts.outputPath = strings.TrimSpace(opts.outputPath)
	opts.eventsPath = strings.TrimSpace(opts.eventsPath)
	opts.conversationDir = strings.TrimSpace(opts.conversationDir)
	return opts, fs.Args(), nil
}

func runRequestMode(requestPath string, outputPath string, events toolloop.EventSink, overrides requestOverrides) error {
	data, err := readInput(requestPath)
	if err != nil {
		return err
	}
	var req toolloop.Request
	if len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &req); err != nil {
			return fmt.Errorf("decode request: %w", err)
		}
	}
	req = mergeConfig(req, overrides)
	result, err := (toolloop.Runner{Stderr: os.Stderr, Events: events}).Run(context.Background(), req)
	return writeResult(outputPath, result, err)
}

func runConversationMode(args []string, conversationDir string, outputPath string, events toolloop.EventSink, overrides requestOverrides) error {
	conversationID, prompt, err := parseConversationArgs(args)
	if err != nil {
		return err
	}
	store := toolloop.NewConversationStore(conversationDir)
	conversation := store.New()
	if conversationID != "" {
		conversation, err = store.Load(conversationID)
		if err != nil {
			return fmt.Errorf("load conversation: %w", err)
		}
	}

	startedAt := time.Now().UTC()
	req := mergeConfig(toolloop.Request{}, overrides)
	req.Messages = append([]toolloop.Message(nil), conversation.Messages...)
	req.Messages = append(req.Messages, toolloop.Message{Role: "user", Content: prompt})
	req.Prompt = ""
	result, runErr := (toolloop.Runner{Stderr: os.Stderr, Events: events}).Run(context.Background(), req)
	result.ConversationID = conversation.ID

	conversation.Model = req.Model
	conversation.Turns = append(conversation.Turns, toolloop.ConversationTurn{
		Prompt:     prompt,
		Status:     result.Status,
		Text:       result.Text,
		Error:      result.Error,
		Iterations: result.Iterations,
		Trace:      result.Trace,
		StartedAt:  startedAt,
		EndedAt:    time.Now().UTC(),
	})
	if runErr == nil && strings.TrimSpace(result.Text) != "" {
		conversation.Messages = append(conversation.Messages, toolloop.Message{Role: "user", Content: prompt})
		conversation.Messages = append(conversation.Messages, toolloop.Message{Role: "assistant", Content: result.Text})
	}
	if saveErr := store.Save(conversation); saveErr != nil && runErr == nil {
		runErr = saveErr
	}
	return writeResult(outputPath, result, runErr)
}

func parseConversationArgs(args []string) (string, string, error) {
	if len(args) == 0 {
		return "", "", fmt.Errorf("missing prompt")
	}
	if args[0] == "resume" {
		if len(args) < 3 {
			return "", "", fmt.Errorf("usage: toolloop resume <conversation_id> -- <prompt>")
		}
		prompt := strings.TrimSpace(strings.Join(stripArgSeparator(args[2:]), " "))
		if prompt == "" {
			return "", "", fmt.Errorf("missing prompt")
		}
		return strings.TrimSpace(args[1]), prompt, nil
	}
	prompt := strings.TrimSpace(strings.Join(stripArgSeparator(args), " "))
	if prompt == "" {
		return "", "", fmt.Errorf("missing prompt")
	}
	return "", prompt, nil
}

func stripArgSeparator(args []string) []string {
	if len(args) > 0 && args[0] == "--" {
		return args[1:]
	}
	return args
}

func writeResult(outputPath string, result toolloop.Result, runErr error) error {
	if shouldWriteResult(result) {
		if writeErr := writeOutput(outputPath, result); writeErr != nil && runErr == nil {
			runErr = writeErr
		}
	} else if runErr == nil {
		if writeErr := writeOutput(outputPath, result); writeErr != nil {
			runErr = writeErr
		}
	}
	return runErr
}

func shouldWriteResult(result toolloop.Result) bool {
	return strings.TrimSpace(result.ConversationID) != "" ||
		strings.TrimSpace(result.Status) != "" ||
		strings.TrimSpace(result.Text) != "" ||
		strings.TrimSpace(result.Error) != "" ||
		result.Iterations > 0 ||
		len(result.Trace) > 0
}

func readInput(path string) ([]byte, error) {
	if path != "" {
		return os.ReadFile(path)
	}
	return io.ReadAll(os.Stdin)
}

func writeOutput(path string, result toolloop.Result) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if path != "" {
		return os.WriteFile(path, data, 0o600)
	}
	_, err = os.Stdout.Write(data)
	return err
}

func openEvents(path string) (toolloop.EventSink, func(), error) {
	if path == "" {
		return nil, func() {}, nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, nil, err
	}
	return toolloop.NewJSONLEventSink(file), func() { _ = file.Close() }, nil
}

func mergeEnv(req toolloop.Request) toolloop.Request {
	env := toolloop.RequestFromEnv(req.Prompt)
	if strings.TrimSpace(req.BaseURL) == "" {
		req.BaseURL = env.BaseURL
	}
	if strings.TrimSpace(req.APIKey) == "" {
		req.APIKey = env.APIKey
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = env.Model
	}
	if strings.TrimSpace(req.System) == "" {
		req.System = env.System
	}
	if strings.TrimSpace(req.Workspace) == "" {
		req.Workspace = env.Workspace
	}
	if req.MaxIterations <= 0 {
		req.MaxIterations = env.MaxIterations
	}
	if req.MaxTokens <= 0 {
		req.MaxTokens = env.MaxTokens
	}
	if req.Temperature == 0 {
		req.Temperature = env.Temperature
	}
	if strings.TrimSpace(req.ModelPromptInstructions) == "" {
		req.ModelPromptInstructions = env.ModelPromptInstructions
	}
	if strings.TrimSpace(req.ModelToolInstructions) == "" {
		req.ModelToolInstructions = env.ModelToolInstructions
	}
	if strings.TrimSpace(req.ModelReasoningFormat) == "" {
		req.ModelReasoningFormat = env.ModelReasoningFormat
	}
	if strings.TrimSpace(req.ModelToolCallFormat) == "" {
		req.ModelToolCallFormat = env.ModelToolCallFormat
	}
	return req
}

func mergeConfig(req toolloop.Request, overrides requestOverrides) toolloop.Request {
	req = mergeEnv(req)
	return applyOverrides(req, overrides)
}

func applyOverrides(req toolloop.Request, overrides requestOverrides) toolloop.Request {
	if overrides.baseURLSet {
		req.BaseURL = overrides.baseURL
	}
	if overrides.apiKeySet {
		req.APIKey = overrides.apiKey
	}
	if overrides.modelSet {
		req.Model = overrides.model
	}
	if overrides.systemSet {
		req.System = overrides.system
	}
	if overrides.workspaceSet {
		req.Workspace = overrides.workspace
	}
	if overrides.maxIterationsSet {
		req.MaxIterations = overrides.maxIterations
	}
	if overrides.maxTokensSet {
		req.MaxTokens = overrides.maxTokens
	}
	if overrides.temperatureSet {
		req.Temperature = overrides.temperature
	}
	if overrides.modelPromptInstructionsSet {
		req.ModelPromptInstructions = overrides.modelPromptInstructions
	}
	if overrides.modelToolInstructionsSet {
		req.ModelToolInstructions = overrides.modelToolInstructions
	}
	if overrides.modelReasoningFormatSet {
		req.ModelReasoningFormat = overrides.modelReasoningFormat
	}
	if overrides.modelToolCallFormatSet {
		req.ModelToolCallFormat = overrides.modelToolCallFormat
	}
	return req
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
