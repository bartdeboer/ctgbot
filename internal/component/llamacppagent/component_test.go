package llamacppagent

import (
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/toolloop"
)

func TestSandboxBaseURLRewritesLocalhost(t *testing.T) {
	t.Parallel()
	got := sandboxBaseURL("http://127.0.0.1:19080/v1")
	want := "http://host.docker.internal:19080/v1"
	if got != want {
		t.Fatalf("sandboxBaseURL() = %q, want %q", got, want)
	}
}

func TestToolloopMessagesIncludesConversationHistory(t *testing.T) {
	t.Parallel()
	history := []coremodel.ThreadMessage{
		{Kind: coremodel.MessageKindUser, Text: "Hello"},
		{Kind: coremodel.MessageKindAgent, Text: "Hi there"},
	}
	messages := toolloopMessages(history, coremodel.ThreadMessage{})
	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "Hello" {
		t.Fatalf("messages[0] = %#v", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Content != "Hi there" {
		t.Fatalf("messages[1] = %#v", messages[1])
	}
}

func TestToolloopMessagesSkipsSystemHistory(t *testing.T) {
	t.Parallel()
	history := []coremodel.ThreadMessage{
		{Kind: coremodel.MessageKindSystem, Text: "system context"},
		{Kind: coremodel.MessageKindUser, Text: "Hello"},
	}
	messages := toolloopMessages(history, coremodel.ThreadMessage{})
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "Hello" {
		t.Fatalf("messages[0] = %#v", messages[0])
	}
}

func TestTextPromptFromMessagesIncludesHistory(t *testing.T) {
	t.Parallel()
	prompt := textPromptFromMessages([]toolloop.Message{{Role: "user", Content: "Hello"}, {Role: "assistant", Content: "Hi"}}, "fallback")
	if !strings.Contains(prompt, "User: Hello") || !strings.Contains(prompt, "Assistant: Hi") {
		t.Fatalf("prompt = %q", prompt)
	}
	if strings.Contains(prompt, "fallback") {
		t.Fatalf("prompt should prefer messages over fallback: %q", prompt)
	}
}

func TestComponentBindConfigSetsWritableGoEnvironment(t *testing.T) {
	t.Parallel()
	config := componentBindConfig(runtimepkg.BindConfig{
		Env: []string{"HOME=/custom", "PATH=/usr/local/go/bin:/usr/bin"},
	}, "/profile/components/llamacppagent/llamacppagent")
	env := map[string]string{}
	for _, value := range config.Env {
		key, val, ok := strings.Cut(value, "=")
		if ok {
			env[key] = val
		}
	}
	for key, want := range map[string]string{
		"HOME":       "/profile/components/llamacppagent/llamacppagent",
		"GOCACHE":    "/profile/components/llamacppagent/llamacppagent/.cache/go-build",
		"GOPATH":     "/profile/components/llamacppagent/llamacppagent/go",
		"GOMODCACHE": "/profile/components/llamacppagent/llamacppagent/go/pkg/mod",
	} {
		if got := env[key]; got != want {
			t.Fatalf("env[%s] = %q, want %q", key, got, want)
		}
	}
	if got := env["PATH"]; got == "" {
		t.Fatalf("PATH should be preserved")
	}
}

func TestRuntimeImageTargetsUseSharedGoNodePythonBase(t *testing.T) {
	t.Parallel()
	targets, err := (&Component{}).RuntimeImageTargets(t.Context())
	if err != nil {
		t.Fatalf("RuntimeImageTargets() error = %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("len(targets) = %d, want 1", len(targets))
	}
	target := targets[0]
	if target.Image != DefaultImage || target.Dockerfile != DefaultDockerfile || !target.NoCache {
		t.Fatalf("target = %#v", target)
	}
	if target.Uses == nil {
		t.Fatalf("target.Uses is nil")
	}
	if target.Uses.Name != "go-node-python-base" || target.Uses.Image != DefaultBaseImage || target.Uses.Dockerfile != DefaultBaseDockerfile {
		t.Fatalf("target.Uses = %#v", target.Uses)
	}
	if target.Uses.Uses != nil {
		t.Fatalf("target.Uses.Uses = %#v, want nil", target.Uses.Uses)
	}
}
