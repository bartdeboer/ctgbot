package applypatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyAddUpdateDelete(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	if _, err := Apply(context.Background(), Request{Workspace: workspace, Patch: `*** Begin Patch
*** Add File: hello.txt
+hello
+world
*** End Patch`}); err != nil {
		t.Fatalf("add: %v", err)
	}
	path := filepath.Join(workspace, "hello.txt")
	if got := readFile(t, path); got != "hello\nworld\n" {
		t.Fatalf("added content = %q", got)
	}
	if _, err := Apply(context.Background(), Request{Workspace: workspace, Patch: `*** Begin Patch
*** Update File: hello.txt
@@
 hello
-world
+ctgbot
*** End Patch`}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := readFile(t, path); got != "hello\nctgbot\n" {
		t.Fatalf("updated content = %q", got)
	}
	if _, err := Apply(context.Background(), Request{Workspace: workspace, Patch: `*** Begin Patch
*** Delete File: hello.txt
*** End Patch`}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still exists or stat error: %v", err)
	}
}

func TestApplyRejectsEscapingWorkspace(t *testing.T) {
	t.Parallel()
	_, err := Apply(context.Background(), Request{Workspace: t.TempDir(), Patch: `*** Begin Patch
*** Add File: ../escape.txt
+bad
*** End Patch`})
	if err == nil || !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("error = %v, want escape error", err)
	}
}

func TestApplyRejectsAmbiguousUpdate(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	path := filepath.Join(workspace, "same.txt")
	if err := os.WriteFile(path, []byte("x\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Apply(context.Background(), Request{Workspace: workspace, Patch: `*** Begin Patch
*** Update File: same.txt
@@
-x
+y
*** End Patch`})
	if err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("error = %v, want ambiguous error", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
