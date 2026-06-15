package system

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func TestThreadExtraInstructionsReadsThreadFile(t *testing.T) {
	root := t.TempDir()
	threadID := modeluuid.New()
	path := filepath.Join(root, "threads", threadID.String(), "extra-instructions.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("\n- Use the thread-specific git alias.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := (&System{StateRoot: root}).ThreadExtraInstructions(context.Background(), threadID)
	if err != nil {
		t.Fatalf("ThreadExtraInstructions() error = %v", err)
	}
	if got != "- Use the thread-specific git alias." {
		t.Fatalf("ThreadExtraInstructions() = %q", got)
	}
}

func TestWriteAndClearThreadExtraInstructions(t *testing.T) {
	root := t.TempDir()
	threadID := modeluuid.New()
	system := &System{StateRoot: root}

	if err := system.WriteThreadExtraInstructions(context.Background(), threadID, []byte("- hello\n")); err != nil {
		t.Fatalf("WriteThreadExtraInstructions() error = %v", err)
	}
	got, err := system.ThreadExtraInstructions(context.Background(), threadID)
	if err != nil {
		t.Fatalf("ThreadExtraInstructions() error = %v", err)
	}
	if got != "- hello" {
		t.Fatalf("ThreadExtraInstructions() = %q", got)
	}
	info, err := os.Stat(filepath.Join(root, "threads", threadID.String(), "extra-instructions.md"))
	if err != nil {
		t.Fatalf("Stat(extra-instructions.md) error = %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o644); got != want {
		t.Fatalf("mode = %o, want %o", got, want)
	}
	if err := system.ClearThreadExtraInstructions(context.Background(), threadID); err != nil {
		t.Fatalf("ClearThreadExtraInstructions() error = %v", err)
	}
	got, err = system.ThreadExtraInstructions(context.Background(), threadID)
	if err != nil {
		t.Fatalf("ThreadExtraInstructions(after clear) error = %v", err)
	}
	if got != "" {
		t.Fatalf("ThreadExtraInstructions(after clear) = %q, want empty", got)
	}
}

func TestThreadExtraInstructionsMissingFileIsEmpty(t *testing.T) {
	got, err := (&System{StateRoot: t.TempDir()}).ThreadExtraInstructions(context.Background(), modeluuid.New())
	if err != nil {
		t.Fatalf("ThreadExtraInstructions() error = %v", err)
	}
	if strings.TrimSpace(got) != "" {
		t.Fatalf("ThreadExtraInstructions() = %q, want empty", got)
	}
}
