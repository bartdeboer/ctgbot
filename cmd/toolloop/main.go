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
	requestPath := flag.String("request", "", "path to JSON toolloop request; stdin is used when empty")
	outputPath := flag.String("output", "", "path to write JSON result; stdout is used when empty")
	eventsPath := flag.String("events", "", "path to write JSONL event stream")
	conversationDir := flag.String("conversation-dir", "", "directory for toolloop conversation JSON files")
	flag.Parse()

	events, closeEvents, err := openEvents(strings.TrimSpace(*eventsPath))
	if err != nil {
		return err
	}
	defer closeEvents()

	if strings.TrimSpace(*requestPath) != "" || len(flag.Args()) == 0 {
		return runRequestMode(strings.TrimSpace(*requestPath), strings.TrimSpace(*outputPath), events)
	}
	return runConversationMode(flag.Args(), strings.TrimSpace(*conversationDir), strings.TrimSpace(*outputPath), events)
}

func runRequestMode(requestPath string, outputPath string, events toolloop.EventSink) error {
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
	req = mergeEnv(req)
	result, err := (toolloop.Runner{Stderr: os.Stderr, Events: events}).Run(context.Background(), req)
	return writeResult(outputPath, result, err)
}

func runConversationMode(args []string, conversationDir string, outputPath string, events toolloop.EventSink) error {
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
	req := mergeEnv(toolloop.Request{})
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
	return req
}
