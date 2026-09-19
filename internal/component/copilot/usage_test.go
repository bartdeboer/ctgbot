package copilot

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/component"
)

func TestUsageSnapshotFileAndCleanup(t *testing.T) {
	c, f, turn := newTestComponent(t)
	var outputPath string
	f.runtime.onExec = func(out io.Writer, args []string) error {
		for i, arg := range args {
			if arg == "--usage-output-file" {
				outputPath = args[i+1]
			}
		}
		if outputPath == "" {
			t.Fatal("missing usage output flag")
		}
		if err := os.WriteFile(outputPath, []byte(`{"modelMetrics":{"m":{"usage":{"inputTokens":100,"cacheReadTokens":80,"cacheWriteTokens":0,"outputTokens":5}}}}`), 0600); err != nil {
			t.Fatal(err)
		}
		id, _ := selectedID(args)
		_, err := fmt.Fprintf(out, "{\"type\":\"assistant.message\",\"data\":{\"messageId\":\"m\",\"content\":\"done\"}}\n{\"type\":\"result\",\"sessionId\":%q,\"exitCode\":0}\n", id)
		return err
	}
	result, err := c.HandleTurn(t.Context(), component.Turn{Thread: turn.thread, Prompt: "test", Runtime: turn})
	if err != nil || result == nil || result.Final.Usage.InputTokens == nil || *result.Final.Usage.InputTokens != 100 || *result.Final.Usage.CachedInputTokens != 80 {
		t.Fatal(result, err)
	}
	if !strings.Contains(result.Final.Usage.Scope, "not a last-turn delta") {
		t.Fatal(result.Final.Usage)
	}
	if _, err := os.Stat(filepath.Dir(outputPath)); !os.IsNotExist(err) {
		t.Fatal("usage staging not removed", err)
	}
}
func TestUsageFileMissingInvalidAndSymlink(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "usage.json")
	if readUsage(path).InputTokens != nil {
		t.Fatal("missing usage")
	}
	for _, body := range []string{`{}`, `{`, `{"modelMetrics":{"m":{"usage":{"inputTokens":-1}}}}`, strings.Repeat("x", 1024*1024+1)} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if readUsage(path).InputTokens != nil {
			t.Fatal("invalid usage accepted")
		}
	}
	os.Remove(path)
	target := filepath.Join(root, "target")
	os.WriteFile(target, []byte(`{"modelMetrics":{"m":{"usage":{"inputTokens":4}}}}`), 0600)
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if readUsage(path).InputTokens != nil {
		t.Fatal("followed symlink")
	}
	u := parseUsage([]byte(`{"modelMetrics":{"m":{"usage":{"inputTokens":0,"cacheReadTokens":0,"cacheWriteTokens":0,"outputTokens":0}}}}`))
	if u.InputTokens == nil || *u.InputTokens != 0 {
		t.Fatal("explicit zero lost")
	}
}
