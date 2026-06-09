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
	viewerThreadID := mustParseTheaterFixtureUUID(t, "00000000-0000-0000-0000-000000000101")
	qwenThreadID := mustParseTheaterFixtureUUID(t, "00000000-0000-0000-0000-000000000102")
	claudeThreadID := mustParseTheaterFixtureUUID(t, "00000000-0000-0000-0000-000000000103")
	codexThreadID := mustParseTheaterFixtureUUID(t, "00000000-0000-0000-0000-000000000104")
	base := testRequest(viewerThreadID)

	lab, _, err := c.store.createTheater(ctx, "qwen-parser-lab", "/workspace/theaters/qwen-parser-lab")
	if err != nil {
		t.Fatal(err)
	}
	orchantic, _, err := c.store.createTheater(ctx, "orchantic-logo", "/workspace/theaters/orchantic-logo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.store.subscribe(ctx, lab, viewerThreadID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.store.subscribe(ctx, orchantic, viewerThreadID); err != nil {
		t.Fatal(err)
	}

	messages := []messageRecord{
		{
			ID:         "msg-001",
			TheaterID:  lab.ID,
			ThreadID:   claudeThreadID.String(),
			ActorID:    "thread:claude-2",
			ActorLabel: "Claude #1",
			Text:       "Parser image cf23967 completed. Final summaries now land as assistant content after tool calls.",
			CreatedAt:  time.Date(2026, 6, 9, 9, 0, 0, 0, time.UTC),
		},
		{
			ID:         "msg-002",
			TheaterID:  lab.ID,
			ThreadID:   codexThreadID.String(),
			ActorID:    "thread:llamacpp-1",
			ActorLabel: "llamacpp 1",
			Text:       "New image available: ghcr.io/bartdeboer/llama-cpp:server-cuda-tagged-thinking-tools-168643697",
			CreatedAt:  time.Date(2026, 6, 9, 9, 7, 0, 0, time.UTC),
		},
		{
			ID:         "msg-003",
			TheaterID:  lab.ID,
			ThreadID:   qwenThreadID.String(),
			ActorID:    "thread:qwen-1",
			ActorLabel: "qwen 1",
			Text:       "Smoke task passed: word_stats_168643697 builds, tests pass, and CLI output matched expected values.",
			CreatedAt:  time.Date(2026, 6, 9, 9, 13, 0, 0, time.UTC),
		},
		{
			ID:         "msg-004",
			TheaterID:  lab.ID,
			ThreadID:   codexThreadID.String(),
			ActorID:    "thread:ctgbot-2",
			ActorLabel: "ctgbot 2",
			Text: strings.Join([]string{
				"Files made available:",
				"- /workspace/theaters/qwen-parser-lab/artifacts/qwen-events.jsonl",
				"- /workspace/theaters/qwen-parser-lab/artifacts/qwen-result.json",
				"Artifact published:",
				"- /workspace/theaters/qwen-parser-lab/design/orchantic/logo-concept-02.png",
			}, "\n"),
			CreatedAt: time.Date(2026, 6, 9, 9, 21, 0, 0, time.UTC),
		},
	}
	if err := c.store.db.WithContext(ctx).Create(&messages).Error; err != nil {
		t.Fatal(err)
	}

	assertTheaterFixture(t, "list.txt", runTheaterFixtureCommand(t, ctx, engine, base, []string{Type, "list"}))
	assertTheaterFixture(t, "status-qwen-parser-lab.txt", runTheaterFixtureCommand(t, ctx, engine, base, []string{Type, "qwen-parser-lab", "status"}))

	updates, err := c.NewUpdates(ctx, component.UpdateRequest{ThreadID: viewerThreadID})
	if err != nil {
		t.Fatal(err)
	}
	assertTheaterFixture(t, "updates-before-read.txt", formatTheaterFixtureUpdates(updates))
	assertTheaterFixture(t, "read-qwen-parser-lab.txt", runTheaterFixtureCommand(t, ctx, engine, base, []string{Type, "qwen-parser-lab", "read", "--limit", "10"}))
	assertTheaterFixture(t, "status-after-read.txt", runTheaterFixtureCommand(t, ctx, engine, base, []string{Type, "status"}))
}

func mustParseTheaterFixtureUUID(t *testing.T, value string) modeluuid.UUID {
	t.Helper()
	id, err := modeluuid.Parse(value)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", value, err)
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
		lines = append(lines, fmt.Sprintf("- %s: %s (%d %ss)", update.Source, update.Ref, update.Count, kind))
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
