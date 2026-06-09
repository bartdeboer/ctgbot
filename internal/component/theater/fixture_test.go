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
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func TestTheaterFixtureMainSnapshot(t *testing.T) {
	ctx := context.Background()
	c := newTestComponent(t)
	engine := newTestEngine(t, c)
	viewerThreadID := mustParseTheaterFixtureUUID(t, "00000000-0000-0000-0000-000000000101")
	qwenThreadID := mustParseTheaterFixtureUUID(t, "00000000-0000-0000-0000-000000000102")
	base := testRequest(viewerThreadID)

	lab, _, err := c.store.createTheater(ctx, "qwen-parser-lab", "/workspace/theaters/qwen-parser-lab", qwenThreadID)
	if err != nil {
		t.Fatal(err)
	}
	orchantic, _, err := c.store.createTheater(ctx, "orchantic-logo", "/workspace/theaters/orchantic-logo", modeluuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.store.subscribe(ctx, lab, viewerThreadID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.store.subscribe(ctx, orchantic, viewerThreadID); err != nil {
		t.Fatal(err)
	}

	messages := []coremodel.ThreadMessage{
		{
			ID:         mustParseTheaterFixtureUUID(t, "00000000-0000-0000-0000-000000001001"),
			ThreadID:   qwenThreadID,
			ActorID:    "thread:claude-2",
			ActorLabel: "Claude #1",
			Direction:  coremodel.MessageDirectionInbound,
			Role:       coremodel.MessageRoleUser,
			Kind:       coremodel.MessageKindMessage,
			Text:       "Parser image cf23967 completed. Final summaries now land as assistant content after tool calls.",
			CreatedAt:  time.Date(2026, 6, 9, 9, 0, 0, 0, time.UTC),
		},
		{
			ID:         mustParseTheaterFixtureUUID(t, "00000000-0000-0000-0000-000000001002"),
			ThreadID:   qwenThreadID,
			ActorID:    "thread:llamacpp-1",
			ActorLabel: "llamacpp 1",
			Direction:  coremodel.MessageDirectionInbound,
			Role:       coremodel.MessageRoleUser,
			Kind:       coremodel.MessageKindMessage,
			Text:       "New image available: ghcr.io/bartdeboer/llama-cpp:server-cuda-tagged-thinking-tools-168643697",
			CreatedAt:  time.Date(2026, 6, 9, 9, 7, 0, 0, time.UTC),
		},
		{
			ID:         mustParseTheaterFixtureUUID(t, "00000000-0000-0000-0000-000000001003"),
			ThreadID:   qwenThreadID,
			ActorID:    "thread:qwen-1",
			ActorLabel: "qwen 1",
			Direction:  coremodel.MessageDirectionInbound,
			Role:       coremodel.MessageRoleUser,
			Kind:       coremodel.MessageKindMessage,
			Text:       "Smoke task passed: word_stats_168643697 builds, tests pass, and CLI output matched expected values.",
			CreatedAt:  time.Date(2026, 6, 9, 9, 13, 0, 0, time.UTC),
		},
		{
			ID:         mustParseTheaterFixtureUUID(t, "00000000-0000-0000-0000-000000001004"),
			ThreadID:   qwenThreadID,
			ActorID:    "thread:ctgbot-2",
			ActorLabel: "ctgbot 2",
			Direction:  coremodel.MessageDirectionInbound,
			Role:       coremodel.MessageRoleUser,
			Kind:       coremodel.MessageKindMessage,
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
	for i := range messages {
		if err := c.storage.Messages().Append(ctx, &messages[i]); err != nil {
			t.Fatal(err)
		}
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
