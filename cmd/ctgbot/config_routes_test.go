package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/appstate"
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

	router := clir.New()
	registerConfigRoutes(router, local, global)

	if err := router.Run(context.Background(), []string{"config", "--set-docker-image", "ctgbot:test"}); err != nil {
		t.Fatalf("Run root setter: %v", err)
	}
	if got := local.GetString("docker.image", ""); got != "ctgbot:test" {
		t.Fatalf("docker.image = %q, want %q", got, "ctgbot:test")
	}

	if err := router.Run(context.Background(), []string{"config", "chat", "123", "--set-enabled", "true"}); err != nil {
		t.Fatalf("Run chat setter: %v", err)
	}

	state, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), local)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	if !state.ChatEnabled(123) {
		t.Fatalf("expected chat 123 to be enabled")
	}

	output := captureStdout(t, func() {
		if err := router.Run(context.Background(), []string{"config"}); err != nil {
			t.Fatalf("Run config show: %v", err)
		}
	})
	if !strings.Contains(output, "Current config:") {
		t.Fatalf("expected config output, got %q", output)
	}
	if strings.Contains(output, "codex.full_auto") {
		t.Fatalf("did not expect dead codex.full_auto config in output: %q", output)
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
