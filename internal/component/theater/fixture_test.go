package theater

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func TestTheaterFixtureMainSnapshot(t *testing.T) {
	ctx := context.Background()
	c := newTestComponent(t)
	engine := newTestEngine(t, c)
	viewer := createTestThreadWithID(t, ctx, c.storage, fixtureUUID(t, "00000000-0000-0000-0000-000000000101"), "Codex #1", fixtureUUID(t, "00000000-0000-0000-0000-000000000102"), "ctgbot 2")
	qwenLab := createTestThreadWithID(t, ctx, c.storage, fixtureUUID(t, "00000000-0000-0000-0000-000000000201"), "Theaters", fixtureUUID(t, "00000000-0000-0000-0000-000000000202"), "qwen-parser-lab")
	orchantic := createTestThreadWithID(t, ctx, c.storage, fixtureUUID(t, "00000000-0000-0000-0000-000000000301"), "Theaters", fixtureUUID(t, "00000000-0000-0000-0000-000000000302"), "orchantic-logo")
	base := testRequest(viewer.ID)

	if _, err := c.store.subscribe(ctx, viewer.ID, qwenLab.ID, qwenLab.Label); err != nil {
		t.Fatal(err)
	}
	if _, err := c.store.subscribe(ctx, viewer.ID, orchantic.ID, orchantic.Label); err != nil {
		t.Fatal(err)
	}

	appendTestMessage(t, ctx, c.storage, qwenLab, "Claude #1", "Parser image cf23967 completed. Final summaries now land as assistant content after tool calls.", time.Date(2026, 6, 9, 9, 0, 0, 0, time.UTC))
	appendTestMessage(t, ctx, c.storage, qwenLab, "llamacpp 1", "New image available: ghcr.io/bartdeboer/llama-cpp:server-cuda-tagged-thinking-tools-168643697", time.Date(2026, 6, 9, 9, 7, 0, 0, time.UTC))
	appendTestMessage(t, ctx, c.storage, qwenLab, "qwen 1", "Smoke task passed: word_stats_168643697 builds, tests pass, and CLI output matched expected values.", time.Date(2026, 6, 9, 9, 13, 0, 0, time.UTC))
	appendTestMessage(t, ctx, c.storage, qwenLab, "ctgbot 2", strings.Join([]string{
		"Files made available:",
		"- /workspace/theaters/qwen-parser-lab/artifacts/qwen-events.jsonl",
		"- /workspace/theaters/qwen-parser-lab/artifacts/qwen-result.json",
		"Artifact published:",
		"- /workspace/theaters/qwen-parser-lab/design/orchantic/logo-concept-02.png",
	}, "\n"), time.Date(2026, 6, 9, 9, 21, 0, 0, time.UTC))

	assertTheaterFixture(t, "list.txt", runTheaterFixtureCommand(t, ctx, engine, base, []string{Type, "list"}))
	assertTheaterFixture(t, "status-qwen-parser-lab.txt", runTheaterFixtureCommand(t, ctx, engine, base, []string{Type, qwenLab.ID.String(), "status"}))

	updates, err := c.NewUpdates(ctx, component.UpdateRequest{ThreadID: viewer.ID})
	if err != nil {
		t.Fatal(err)
	}
	assertTheaterFixture(t, "updates-before-read.txt", formatTheaterFixtureUpdates(updates))
	assertTheaterFixture(t, "read-qwen-parser-lab.txt", runTheaterFixtureCommand(t, ctx, engine, base, []string{Type, qwenLab.ID.String(), "read", "--limit", "10"}))
	assertTheaterFixture(t, "status-after-read.txt", runTheaterFixtureCommand(t, ctx, engine, base, []string{Type, "status"}))
}

func fixtureUUID(t *testing.T, value string) modeluuid.UUID {
	t.Helper()
	id, err := modeluuid.Parse(value)
	if err != nil {
		t.Fatalf("parse fixture uuid %q: %v", value, err)
	}
	return id
}

func runTheaterFixtureCommand(t *testing.T, ctx context.Context, engine interface {
	Run(context.Context, commandengine.Request, []string) (commandengine.Result, error)
}, base commandengine.Request, argv []string) string {
	t.Helper()
	result, err := engine.Run(ctx, base, argv)
	if err != nil {
		t.Fatalf("run %v: %v", argv, err)
	}
	return result.Text
}

func formatTheaterFixtureUpdates(updates []component.UpdateNotice) string {
	if len(updates) == 0 {
		return "Updates:\n- none"
	}
	lines := []string{"Updates:"}
	for _, update := range updates {
		kind := strings.TrimSpace(update.Kind)
		if kind == "" {
			kind = "update"
		}
		lines = append(lines, fmt.Sprintf("- %s: %s (%d %ss)", update.Source, update.Label, update.Count, kind))
	}
	return strings.Join(lines, "\n")
}

func assertTheaterFixture(t *testing.T, name string, got string) {
	t.Helper()
	got = strings.TrimSpace(got) + "\n"
	path := filepath.Join("testdata", name)
	if os.Getenv("CTGBOT_UPDATE_TESTDATA") == "1" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("update fixture: %v", err)
		}
		return
	}
	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	want := strings.TrimSpace(string(wantBytes)) + "\n"
	if got != want {
		t.Fatalf("fixture %s mismatch\n--- got ---\n%s--- want ---\n%s", name, got, want)
	}
}
