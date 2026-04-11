package configsetters

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/clisetter"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func TestConfigSettersRegisterRoutes(t *testing.T) {
	root := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(prev) }()

	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("WriteFile go.mod: %v", err)
	}

	local, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("NewCwd: %v", err)
	}
	global, err := clistate.NewGlobal("ctgbot", "config")
	if err != nil {
		t.Fatalf("NewGlobal: %v", err)
	}
	state, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), local)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	target := NewConfigSetters(state, local, global)
	router := clir.New()
	router.Routes(func(b *clir.Builder) {
		b.Route("config", func(b *clir.Builder) {
			if err := clisetter.New(target).RegisterRoutes(b); err != nil {
				t.Fatalf("RegisterRoutes: %v", err)
			}
		})
	})

	if err := router.Run(context.Background(), []string{"config", "--set-docker-image", "ctgbot:test"}); err != nil {
		t.Fatalf("Run root setter: %v", err)
	}
	if got := local.GetString("docker.image", ""); got != "ctgbot:test" {
		t.Fatalf("docker.image = %q, want %q", got, "ctgbot:test")
	}

	entry, err := state.EnsureProviderChat("telegram", "123", "Test Chat")
	if err != nil {
		t.Fatalf("EnsureProviderChat: %v", err)
	}

	if err := router.Run(context.Background(), []string{"config", "chat", entry.ID.String(), "--set-enabled", "true"}); err != nil {
		t.Fatalf("Run chat setter: %v", err)
	}
	if !state.ChatEnabledByID(entry.ID) {
		t.Fatalf("expected chat 123 to be enabled")
	}

	if err := router.Run(context.Background(), []string{"config", "chat", entry.ID.String(), "hostbridge", "origin", "--set-command", "git"}); err != nil {
		t.Fatalf("Run hostbridge alias command setter: %v", err)
	}
	if err := router.Run(context.Background(), []string{"config", "chat", entry.ID.String(), "hostbridge", "origin", "--set-dir", "/repo"}); err != nil {
		t.Fatalf("Run hostbridge alias setter: %v", err)
	}
	command := state.ChatHostbridgeAllowedCommandsByID(entry.ID)["origin"]
	if command.Dir != "/repo" {
		t.Fatalf("command.Dir = %q, want %q", command.Dir, "/repo")
	}
}
