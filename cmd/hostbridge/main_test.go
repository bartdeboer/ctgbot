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

	req, err := buildSendFileRequest(path, "Weekly report", "")
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

	if _, err := buildSendFileRequest(path, "", ""); err == nil {
		t.Fatalf("expected missing runtime identity error")
	}
}

func TestBuildSendTextRequestPlainDefaultsToTextPlain(t *testing.T) {
	t.Setenv("CTGBOT_SANDBOX_ID", "thread-456")
	req, err := buildSendTextRequest("hello\nworld\n", "", false, "", "")
	if err != nil {
		t.Fatalf("build sendtext request: %v", err)
	}
	if req.Op != hostbridge.OpSendText {
		t.Fatalf("req.Op = %q, want %q", req.Op, hostbridge.OpSendText)
	}
	if req.SandboxID != "thread-456" {
		t.Fatalf("req.SandboxID = %q", req.SandboxID)
	}
	if req.Text != "hello\nworld\n" {
		t.Fatalf("req.Text = %q", req.Text)
	}
	if req.ContentType != "text/plain" {
		t.Fatalf("req.ContentType = %q", req.ContentType)
	}
	if req.Fenced {
		t.Fatalf("req.Fenced = true, want false")
	}
}

func TestBuildSendTextRequestSyntaxFencesText(t *testing.T) {
	t.Setenv("CTGBOT_SANDBOX_ID", "thread-456")
	req, err := buildSendTextRequest("git diff --stat\n", "text/plain", false, "", "diff")
	if err != nil {
		t.Fatalf("build sendtext request: %v", err)
	}
	if !req.Fenced {
		t.Fatalf("req.Fenced = false, want true")
	}
	if req.Language != "diff" {
		t.Fatalf("req.Language = %q", req.Language)
	}
	want := "```diff\ngit diff --stat\n```"
	if req.Text != want {
		t.Fatalf("req.Text = %q, want %q", req.Text, want)
	}
}

func TestBuildSendTextRequestLegacyLanguageImpliesFence(t *testing.T) {
	t.Setenv("CTGBOT_SANDBOX_ID", "thread-456")
	req, err := buildSendTextRequest("git diff --stat\n", "text/plain", false, "diff", "")
	if err != nil {
		t.Fatalf("build sendtext request: %v", err)
	}
	if !req.Fenced {
		t.Fatalf("req.Fenced = false, want true")
	}
	if req.Language != "diff" {
		t.Fatalf("req.Language = %q", req.Language)
	}
}

func TestBuildSendTextRequestRejectsSyntaxWithMarkdown(t *testing.T) {
	t.Setenv("CTGBOT_SANDBOX_ID", "thread-456")
	if _, err := buildSendTextRequest("# hi\n", "text/markdown", false, "", "go"); err == nil {
		t.Fatalf("expected syntax+markdown error")
	}
}

func TestBuildSendFileRequestPreservesContentType(t *testing.T) {
	t.Setenv("CTGBOT_SANDBOX_ID", "thread-456")
	path := filepath.Join(t.TempDir(), "report.md")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	req, err := buildSendFileRequest(path, "", "text/markdown")
	if err != nil {
		t.Fatalf("build sendfile request: %v", err)
	}
	if req.ContentType != "text/markdown" {
		t.Fatalf("req.ContentType = %q", req.ContentType)
	}
}
