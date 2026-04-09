package codexengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/appconfig"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/go-clistate"
)

func TestEnsureConversationCodexHomeWritesPosixModelInstructionsPath(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.PersistString("docker.container_home_path", `\codex-home`); err != nil {
		t.Fatalf("persist container home path: %v", err)
	}

	cfg, err := appconfig.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	homeDir := filepath.Join(root, "chat", ".codex")
	if err := ensureConversationCodexHome(cfg, homeDir, "/codex-home", "/workspace", "bootstrap"); err != nil {
		t.Fatalf("ensure conversation codex home: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(homeDir, "config.toml"))
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}

	text := string(body)
	if !strings.Contains(text, `model_instructions_file = "/codex-home/ctgbot-bootstrap.md"`) {
		t.Fatalf("config.toml does not contain normalized model_instructions_file:\n%s", text)
	}
	if strings.Contains(text, `\codex-home\ctgbot-bootstrap.md`) {
		t.Fatalf("config.toml still contains a windows-style model_instructions_file:\n%s", text)
	}
	if !strings.Contains(text, `writable_roots = ["/workspace", "/codex-home", "/tmp"]`) {
		t.Fatalf("config.toml does not contain expanded writable roots:\n%s", text)
	}
}

func TestExtractCodexThreadID(t *testing.T) {
	t.Parallel()

	jsonl := strings.Join([]string{
		`{"type":"turn.started"}`,
		`{"type":"thread.started","thread_id":"0199a213-81c0-7800-8aa1-bbab2a035a53"}`,
		`{"type":"turn.completed"}`,
	}, "\n")

	got := extractCodexThreadID(jsonl)
	want := "0199a213-81c0-7800-8aa1-bbab2a035a53"
	if got != want {
		t.Fatalf("extractCodexThreadID() = %q, want %q", got, want)
	}
}

func TestExtractCodexThreadIDIgnoresInvalidLines(t *testing.T) {
	t.Parallel()

	jsonl := strings.Join([]string{
		`not-json`,
		`{"type":"item.started"}`,
		`{"type":"thread.started","thread_id":"abc-123"}`,
	}, "\n")

	if got := extractCodexThreadID(jsonl); got != "abc-123" {
		t.Fatalf("extractCodexThreadID() = %q, want %q", got, "abc-123")
	}
}

func TestSetupEnvironmentWritesManagedFiles(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg, err := appconfig.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	sharedAuth := filepath.Join(root, ".ctgbot", ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(sharedAuth), 0o755); err != nil {
		t.Fatalf("mkdir shared auth dir: %v", err)
	}
	if err := os.WriteFile(sharedAuth, []byte(`{"token":"x"}`), 0o600); err != nil {
		t.Fatalf("write shared auth: %v", err)
	}

	exec := &SessionExecutor{Config: cfg}
	sbx := &sandboxengine.Sandbox{
		ProfileDir:            filepath.Join(root, "chat", ".codex"),
		ContainerHome:         "/codex-home",
		ContainerWorkspace:    "/workspace",
		DeveloperInstructions: "bootstrap text",
	}

	if err := exec.SetupEnvironment(t.Context(), sbx); err != nil {
		t.Fatalf("setup environment: %v", err)
	}

	for _, name := range []string{"auth.json", "config.toml", "ctgbot-bootstrap.md"} {
		if _, err := os.Stat(filepath.Join(sbx.ProfileDir, name)); err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
	}

	body, err := os.ReadFile(filepath.Join(sbx.ProfileDir, "config.toml"))
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `writable_roots = ["/workspace", "/codex-home", "/tmp"]`) {
		t.Fatalf("config.toml does not contain expanded writable roots:\n%s", text)
	}
}
