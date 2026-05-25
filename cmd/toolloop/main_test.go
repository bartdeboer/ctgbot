package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/toolloop"
)

func TestParseConversationArgs(t *testing.T) {
	t.Parallel()
	conversationID, prompt, err := parseConversationArgs([]string{"resume", "conv-1", "--", "hello", "world"})
	if err != nil {
		t.Fatal(err)
	}
	if conversationID != "conv-1" || prompt != "hello world" {
		t.Fatalf("conversationID=%q prompt=%q", conversationID, prompt)
	}
}

func TestRunConversationModeCreatesAndResumesConversation(t *testing.T) {
	conversationDir := t.TempDir()
	outputPath := filepath.Join(t.TempDir(), "result.json")
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requestCount++
		var body struct {
			Messages []toolloop.Message `json:"messages"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		switch requestCount {
		case 1:
			if len(body.Messages) != 1 || body.Messages[0].Content != "hello" {
				t.Fatalf("first messages = %#v", body.Messages)
			}
		case 2:
			if len(body.Messages) != 3 || body.Messages[0].Content != "hello" || body.Messages[1].Content != "first" || body.Messages[2].Content != "again" {
				t.Fatalf("second messages = %#v", body.Messages)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		content := "first"
		if requestCount == 2 {
			content = "second"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": content}}}})
	}))
	defer server.Close()

	t.Setenv("TOOLLOOP_BASE_URL", server.URL)
	t.Setenv("TOOLLOOP_MODEL", "qwen")

	if err := runConversationMode([]string{"hello"}, conversationDir, outputPath, nil, requestOverrides{}); err != nil {
		t.Fatalf("first runConversationMode() error = %v", err)
	}
	result := readResult(t, outputPath)
	if result.ConversationID == "" || result.Text != "first" {
		t.Fatalf("first result = %#v", result)
	}

	if err := runConversationMode([]string{"resume", result.ConversationID, "again"}, conversationDir, outputPath, nil, requestOverrides{}); err != nil {
		t.Fatalf("second runConversationMode() error = %v", err)
	}
	result = readResult(t, outputPath)
	if result.Text != "second" {
		t.Fatalf("second result = %#v", result)
	}

	conversationData, err := os.ReadFile(filepath.Join(conversationDir, result.ConversationID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var conversation toolloop.Conversation
	if err := json.Unmarshal(conversationData, &conversation); err != nil {
		t.Fatal(err)
	}
	if len(conversation.Messages) != 4 || len(conversation.Turns) != 2 {
		t.Fatalf("conversation = %#v", conversation)
	}
}

func TestRunArgsUsesCLIConfigFlags(t *testing.T) {
	conversationDir := t.TempDir()
	outputPath := filepath.Join(t.TempDir(), "result.json")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Model       string             `json:"model"`
			MaxTokens   int                `json:"max_tokens"`
			Temperature float64            `json:"temperature"`
			Messages    []toolloop.Message `json:"messages"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Model != "cli-model" || body.MaxTokens != 1234 || body.Temperature != 0.25 {
			t.Fatalf("request config = model %q max_tokens %d temperature %g", body.Model, body.MaxTokens, body.Temperature)
		}
		if len(body.Messages) != 1 || body.Messages[0].Content != "hello from flags" {
			t.Fatalf("messages = %#v", body.Messages)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": "ok"}}}})
	}))
	defer server.Close()

	t.Setenv("TOOLLOOP_BASE_URL", "http://bad-backend.invalid")
	t.Setenv("TOOLLOOP_MODEL", "env-model")

	if err := runArgs([]string{
		"--base-url", server.URL,
		"--model", "cli-model",
		"--max-tokens", "1234",
		"--temperature", "0.25",
		"--conversation-dir", conversationDir,
		"--output", outputPath,
		"--",
		"hello from flags",
	}); err != nil {
		t.Fatalf("runArgs() error = %v", err)
	}
	result := readResult(t, outputPath)
	if result.Text != "ok" || result.ConversationID == "" {
		t.Fatalf("result = %#v", result)
	}
}

func TestParseCLIRejectsConflictingBaseURLAliases(t *testing.T) {
	_, _, err := parseCLI([]string{"--base-url", "http://one", "--llama-backend", "http://two", "--", "hello"})
	if err == nil {
		t.Fatal("expected conflicting base URL aliases to fail")
	}
}

func readResult(t *testing.T, path string) toolloop.Result {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result toolloop.Result
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
