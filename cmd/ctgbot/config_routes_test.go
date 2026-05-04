package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appstate "github.com/bartdeboer/ctgbot/internal/appstate"
	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func TestRegisterConfigRoutes_ShowAndMutate(t *testing.T) {
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

	router := clir.New()
	registerConfigRoutes(router, local, global)

	if err := router.Run(context.Background(), []string{"config", "set", "docker.image", "ctgbot:test"}); err != nil {
		t.Fatalf("Run root setter: %v", err)
	}
	if got := local.GetString("docker.image", ""); got != "ctgbot:test" {
		t.Fatalf("docker.image = %q, want %q", got, "ctgbot:test")
	}

	entry, err := state.EnsureProviderChat("telegram", "123", "Test Chat")
	if err != nil {
		t.Fatalf("EnsureProviderChat: %v", err)
	}

	if err := router.Run(context.Background(), []string{"config", "chat", entry.ID.String(), "set", "chat.enabled", "true"}); err != nil {
		t.Fatalf("Run chat setter: %v", err)
	}
	if !state.Chat(entry.ID).Enabled() {
		t.Fatalf("expected chat 123 to be enabled")
	}
	if err := router.Run(context.Background(), []string{"config", "chat", entry.ID.String(), "set", "chat.gpus", "all"}); err != nil {
		t.Fatalf("Run chat gpu setter: %v", err)
	}
	if got := state.Chat(entry.ID).GPUs(); got != "all" {
		t.Fatalf("chat gpus = %q, want %q", got, "all")
	}
	if err := router.Run(context.Background(), []string{"config", "chat", entry.ID.String(), "hostbridge", "scaffold", "origin"}); err != nil {
		t.Fatalf("Run hostbridge scaffold: %v", err)
	}
	var commands map[string]hostbridgeserver.AllowedCommand
	key := `chats["` + entry.ID.String() + `"].hostbridge.allowed_commands`
	if !local.GetStruct(key, &commands) {
		t.Fatalf("allowed command skeleton not stored at %s", key)
	}
	if _, ok := commands["origin"]; !ok {
		t.Fatalf("allowed commands = %#v, want origin skeleton", commands)
	}

	getOutput := captureStdout(t, func() {
		if err := router.Run(context.Background(), []string{"config", "chat", entry.ID.String(), "get", "chat.gpus"}); err != nil {
			t.Fatalf("Run chat get: %v", err)
		}
	})
	if !strings.Contains(getOutput, "chat.gpus=all") {
		t.Fatalf("expected chat get output, got %q", getOutput)
	}

	output := captureStdout(t, func() {
		if err := router.Run(context.Background(), []string{"config"}); err != nil {
			t.Fatalf("Run config show: %v", err)
		}
	})
	if !strings.Contains(output, "Current config:") {
		t.Fatalf("expected config output, got %q", output)
	}
	if !strings.Contains(output, "telegram.operators: []int64(nil)") {
		t.Fatalf("expected telegram operators in output, got %q", output)
	}
	if strings.Contains(output, "codex.full_auto") {
		t.Fatalf("did not expect dead codex.full_auto config in output: %q", output)
	}
	if !strings.Contains(output, `gpus="all"`) {
		t.Fatalf("expected chat gpu config in output, got %q", output)
	}
	if !strings.Contains(output, "hostbridge.allowed_commands: names=[origin]") {
		t.Fatalf("expected scaffolded hostbridge alias in output, got %q", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	prev := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = prev }()

	outC := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outC <- buf.String()
	}()

	fn()

	_ = w.Close()
	return <-outC
}
