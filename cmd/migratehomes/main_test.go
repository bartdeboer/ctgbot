package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateHomesDryRunDoesNotCopy(t *testing.T) {
	root := t.TempDir()
	oldHome := filepath.Join(root, "threads", "thread-1", "components", "codex", "codex", "home")
	mustWrite(t, filepath.Join(oldHome, "bin", "tool"), "hello")

	var stdout, stderr bytes.Buffer
	code := run([]string{"--state-root", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run code = %d, stderr = %s, stdout = %s", code, stderr.String(), stdout.String())
	}
	if _, err := os.Stat(filepath.Join(root, "threads", "thread-1", "home", "bin", "tool")); !os.IsNotExist(err) {
		t.Fatalf("dry run created target file, stat err = %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "dry_run=true") || !strings.Contains(got, "would_copy=1") {
		t.Fatalf("unexpected stdout:\n%s", got)
	}
}

func TestMigrateHomesApplyCopiesOldHomeToThreadHome(t *testing.T) {
	root := t.TempDir()
	oldHome := filepath.Join(root, "threads", "thread-1", "components", "codex", "codex", "home")
	mustWrite(t, filepath.Join(oldHome, "README.md"), "readme")
	mustWrite(t, filepath.Join(oldHome, "bin", "tool"), "hello")
	if err := os.Symlink("bin/tool", filepath.Join(oldHome, "tool-link")); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"--state-root", root, "--apply"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run code = %d, stderr = %s, stdout = %s", code, stderr.String(), stdout.String())
	}
	if got := mustRead(t, filepath.Join(root, "threads", "thread-1", "home", "bin", "tool")); got != "hello" {
		t.Fatalf("copied tool = %q", got)
	}
	if got, err := os.Readlink(filepath.Join(root, "threads", "thread-1", "home", "tool-link")); err != nil || got != "bin/tool" {
		t.Fatalf("copied symlink = %q, err = %v", got, err)
	}
	if got := stdout.String(); !strings.Contains(got, "copied=3") || !strings.Contains(got, "conflicts=0") {
		t.Fatalf("unexpected stdout:\n%s", got)
	}
}

func TestMigrateHomesReportsConflictsWithoutOverwrite(t *testing.T) {
	root := t.TempDir()
	oldHome := filepath.Join(root, "threads", "thread-1", "components", "codex", "codex", "home")
	newHome := filepath.Join(root, "threads", "thread-1", "home")
	mustWrite(t, filepath.Join(oldHome, "bin", "tool"), "old")
	mustWrite(t, filepath.Join(newHome, "bin", "tool"), "new")

	var stdout, stderr bytes.Buffer
	code := run([]string{"--state-root", root, "--apply"}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("run code = %d, want 3, stderr = %s, stdout = %s", code, stderr.String(), stdout.String())
	}
	if got := mustRead(t, filepath.Join(newHome, "bin", "tool")); got != "new" {
		t.Fatalf("target overwritten: %q", got)
	}
	if got := stdout.String(); !strings.Contains(got, "conflict: bin/tool") || !strings.Contains(got, "conflicts=1") {
		t.Fatalf("unexpected stdout:\n%s", got)
	}
}

func mustWrite(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
