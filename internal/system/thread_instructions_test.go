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

func TestThreadExtraInstructionsMissingFileIsEmpty(t *testing.T) {
	got, err := (&System{StateRoot: t.TempDir()}).ThreadExtraInstructions(context.Background(), modeluuid.New())
	if err != nil {
		t.Fatalf("ThreadExtraInstructions() error = %v", err)
	}
	if strings.TrimSpace(got) != "" {
		t.Fatalf("ThreadExtraInstructions() = %q, want empty", got)
	}
}
