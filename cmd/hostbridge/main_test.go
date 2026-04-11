package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/hostbridge"
)

func TestBuildSendFileRequest(t *testing.T) {
	t.Setenv("CTGBOT_CHAT_ID", "chat-123")
	t.Setenv("CTGBOT_THREAD_ID", "thread-456")

	path := filepath.Join(t.TempDir(), "report.pdf")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	req, err := buildSendFileRequest(path, "Weekly report")
	if err != nil {
		t.Fatalf("build sendfile request: %v", err)
	}
	if req.Op != hostbridge.OpSendFile {
		t.Fatalf("req.Op = %q, want %q", req.Op, hostbridge.OpSendFile)
	}
	if req.ChatID != "chat-123" {
		t.Fatalf("req.ChatID = %q", req.ChatID)
	}
	if req.ThreadID != "thread-456" {
		t.Fatalf("req.ThreadID = %q", req.ThreadID)
	}
	if req.Filename != "report.pdf" {
		t.Fatalf("req.Filename = %q", req.Filename)
	}
	if req.Caption != "Weekly report" {
		t.Fatalf("req.Caption = %q", req.Caption)
	}
	if string(req.Content) != "hello" {
		t.Fatalf("req.Content = %q", string(req.Content))
	}
}

func TestBuildSendFileRequestRequiresRuntimeIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.pdf")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := buildSendFileRequest(path, ""); err == nil {
		t.Fatalf("expected missing runtime identity error")
	}
}
