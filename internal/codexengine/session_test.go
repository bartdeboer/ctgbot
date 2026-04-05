package codexengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/go-clistate"
	"github.com/bartdeboer/ctgbot/internal/appconfig"
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
	if err := ensureConversationCodexHome(cfg, homeDir, "bootstrap"); err != nil {
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
}
