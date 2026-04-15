package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/hostbridge"
)

func TestBuildSendFileRequest(t *testing.T) {
	t.Setenv("CTGBOT_SANDBOX_ID", "thread-456")

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
	if req.SandboxID != "thread-456" {
		t.Fatalf("req.SandboxID = %q", req.SandboxID)
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
	t.Setenv("CTGBOT_SANDBOX_ID", "")

	path := filepath.Join(t.TempDir(), "report.pdf")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := buildSendFileRequest(path, ""); err == nil {
		t.Fatalf("expected missing runtime identity error")
	}
}
