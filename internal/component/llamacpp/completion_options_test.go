package llamacpp

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/ctgbot/internal/workgate"
)

func completionTestComponent(t *testing.T, handler http.HandlerFunc) (*Component, *stdinBackend) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	backend := &stdinBackend{url: server.URL}
	return &Component{componentConfig: ComponentConfig{ModelPath: "/unused.gguf"}.withDefaults(), backendFactory: stdinFactory{b: backend}, client: server.Client(), inferenceGate: workgate.New()}, backend
}

func parseCompletionTest(t *testing.T, c *Component, args ...string) (commandengine.Request, error) {
	t.Helper()
	router, err := commandengine.NewRouter(c.CommandDefinitions(), commandengine.SourceHostbridge)
	if err != nil {
		t.Fatal(err)
	}
	return router.Parse(t.Context(), commandengine.Request{Context: commandengine.Context{Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}}}}, args)
}

func TestCompletionEmptyAnswerIsNotSuccess(t *testing.T) {
	for _, payload := range []string{
		`{"choices":[{"finish_reason":"length","message":{"content":"","reasoning_content":"SECRET"}}]}`,
		`{"choices":[{"finish_reason":"stop","message":{"content":"  \n"}}]}`,
		`{"choices":[]}`,
	} {
		for _, stdin := range []bool{false, true} {
			c, _ := completionTestComponent(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, payload) })
			result, err := c.handleCompletionCommand(t.Context(), commandengine.Request{Stdin: "SECRET"}, completionCommand{Prompt: "Summarize", Stdin: stdin})
			const want = "model returned no final answer; try --reasoning disabled or a larger --max-tokens limit"
			if err == nil || err.Error() != want || result.Text != "" {
				t.Fatalf("result=%v err=%v", result, err)
			}
		}
	}
}

func TestStdinCompletionProviderErrorDoesNotEchoInput(t *testing.T) {
	c, _ := completionTestComponent(t, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "SECRET document and reasoning", 500) })
	_, err := c.handleCompletionCommand(t.Context(), commandengine.Request{Stdin: "SECRET"}, completionCommand{Prompt: "Summarize", Stdin: true})
	if err == nil || strings.Contains(err.Error(), "SECRET") || !strings.Contains(err.Error(), "withheld") {
		t.Fatalf("err=%v", err)
	}
}

func TestCompletionInvocationOptionsForwarded(t *testing.T) {
	gob.Register(completionCommand{})
	for _, tc := range []struct {
		flags     []string
		tokens    int
		reasoning *bool
	}{
		{nil, 1024, nil},
		{[]string{"--reasoning", "default"}, 1024, nil},
		{[]string{"--reasoning", "disabled", "--max-tokens", "512"}, 512, boolOption(false)},
		{[]string{"--reasoning", "enabled", "--max-tokens", "2048"}, 2048, boolOption(true)},
	} {
		bodyCh := make(chan map[string]any, 1)
		c, _ := completionTestComponent(t, func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			bodyCh <- body
			_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"answer"}}]}`)
		})
		args := append([]string{"completion", "Summarize", "--stdin"}, tc.flags...)
		req, err := parseCompletionTest(t, c, args...)
		if err != nil {
			t.Fatal(err)
		}
		req.Stdin = "document"
		var wire bytes.Buffer
		if err := gob.NewEncoder(&wire).Encode(req); err != nil {
			t.Fatal(err)
		}
		var decoded commandengine.Request
		if err := gob.NewDecoder(&wire).Decode(&decoded); err != nil {
			t.Fatal(err)
		}
		req = decoded
		result, err := c.handleCompletionCommand(t.Context(), req, req.Command.(completionCommand))
		if err != nil || result.Text != "answer" {
			t.Fatalf("result=%v err=%v", result, err)
		}
		body := <-bodyCh
		if body["max_tokens"] != float64(tc.tokens) {
			t.Fatalf("max_tokens=%v", body["max_tokens"])
		}
		kwargs, _ := body["chat_template_kwargs"].(map[string]any)
		if tc.reasoning == nil {
			if _, ok := kwargs["enable_thinking"]; ok {
				t.Fatal("default forced reasoning")
			}
		} else if kwargs["enable_thinking"] != *tc.reasoning {
			t.Fatalf("kwargs=%v", kwargs)
		}
	}
}

func boolOption(v bool) *bool { return &v }

func TestCompletionInvalidOptionsBeforeBackend(t *testing.T) {
	c := &Component{}
	for _, flags := range [][]string{
		{"--max-tokens", "0"}, {"--max-tokens", "-1"}, {"--max-tokens", "oops"},
		{"--max-tokens", "999999999999999999999999"}, {"--reasoning", "unknown"},
	} {
		if _, err := parseCompletionTest(t, c, append([]string{"completion", "Summarize"}, flags...)...); err == nil {
			t.Fatalf("accepted flags %v", flags)
		}
	}
	for _, cmd := range []completionCommand{
		{Prompt: "test", MaxTokens: 0, MaxTokensSet: true},
		{Prompt: "test", Reasoning: component.ReasoningMode("invalid")},
	} {
		_, err := (*Component)(nil).handleCompletionCommand(t.Context(), commandengine.Request{}, cmd)
		if err == nil || (!strings.Contains(err.Error(), "must be")) {
			t.Fatalf("typed validation err=%v", err)
		}
	}
}
