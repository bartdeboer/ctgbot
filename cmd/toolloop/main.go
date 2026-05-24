package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

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
	flag.Parse()
	data, err := readInput(strings.TrimSpace(*requestPath))
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
	events, closeEvents, err := openEvents(strings.TrimSpace(*eventsPath))
	if err != nil {
		return err
	}
	defer closeEvents()
	result, err := (toolloop.Runner{Stderr: os.Stderr, Events: events}).Run(context.Background(), req)
	if shouldWriteResult(result) {
		if writeErr := writeOutput(strings.TrimSpace(*outputPath), result); writeErr != nil && err == nil {
			err = writeErr
		}
	} else if err == nil {
		if writeErr := writeOutput(strings.TrimSpace(*outputPath), result); writeErr != nil {
			err = writeErr
		}
	}
	if err != nil {
		return err
	}
	return nil
}

func shouldWriteResult(result toolloop.Result) bool {
	return strings.TrimSpace(result.Status) != "" ||
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
