package theater

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func TestTheaterCreateSubscribePostReadFlow(t *testing.T) {
	ctx := context.Background()
	component := newTestComponent(t)
	engine := newTestEngine(t, component)
	threadID := modeluuid.New()
	base := testRequest(threadID)

	if result, err := engine.Run(ctx, base, []string{Type, "create", "qwen-parser-lab"}); err != nil || result.Text != "theater created: qwen-parser-lab" {
		t.Fatalf("create result=%q err=%v", result.Text, err)
	}
	for _, path := range []string{"README.md", "AGENTS.md", "SKILLS"} {
		if _, err := os.Stat(filepath.Join(component.workspacePath("qwen-parser-lab"), path)); err != nil {
			t.Fatalf("expected theater workspace path %s: %v", path, err)
		}
	}
	if result, err := engine.Run(ctx, base, []string{Type, "qwen-parser-lab", "subscribe"}); err != nil || result.Text != "subscribed: qwen-parser-lab" {
		t.Fatalf("subscribe result=%q err=%v", result.Text, err)
	}
	if _, err := engine.Run(ctx, base, []string{Type, "qwen-parser-lab", "post", "parser", "image", "ready"}); err != nil {
		t.Fatalf("post error = %v", err)
	}

	updates, err := component.NewUpdates(ctx, componentUpdateRequest(threadID))
	if err != nil {
		t.Fatalf("NewUpdates() error = %v", err)
	}
	if len(updates) != 1 || updates[0].Source != Type || updates[0].Label != "qwen-parser-lab" || updates[0].Count != 1 {
		t.Fatalf("updates = %#v, want one theater update", updates)
	}
	status, err := engine.Run(ctx, base, []string{Type, "qwen-parser-lab", "status"})
	if err != nil {
		t.Fatalf("named status error = %v", err)
	}
	if !strings.Contains(status.Text, "unread messages: 1") {
		t.Fatalf("named status = %q, want pending count", status.Text)
	}

	read, err := engine.Run(ctx, base, []string{Type, "qwen-parser-lab", "read"})
	if err != nil {
		t.Fatalf("read error = %v", err)
	}
	if !strings.Contains(read.Text, "tester: parser image ready") {
		t.Fatalf("read = %q, want posted message", read.Text)
	}
	updates, err = component.NewUpdates(ctx, componentUpdateRequest(threadID))
	if err != nil {
		t.Fatalf("NewUpdates after read error = %v", err)
	}
	if len(updates) != 0 {
		t.Fatalf("updates after read = %#v, want none", updates)
	}
}

func TestTheaterPostUsesStdin(t *testing.T) {
	ctx := context.Background()
	component := newTestComponent(t)
	engine := newTestEngine(t, component)
	threadID := modeluuid.New()
	base := testRequest(threadID)
	if _, err := engine.Run(ctx, base, []string{Type, "create", "lab"}); err != nil {
		t.Fatal(err)
	}
	postReq := base
	postReq.Stdin = "hello from stdin"
	if _, err := engine.Run(ctx, postReq, []string{Type, "lab", "post"}); err != nil {
		t.Fatalf("post stdin error = %v", err)
	}
	read, err := engine.Run(ctx, base, []string{Type, "lab", "read"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read.Text, "hello from stdin") {
		t.Fatalf("read = %q, want stdin message", read.Text)
	}
}

func newTestComponent(t *testing.T) *Component {
	t.Helper()
	store, err := openStore(t.TempDir())
	if err != nil {
		t.Fatalf("openStore() error = %v", err)
	}
	return &Component{registration: coremodel.Component{Type: Type, Name: Type}, store: store, workspaceRoot: filepath.Join(t.TempDir(), "theaters")}
}

func newTestEngine(t *testing.T, c *Component) *commandengine.Engine {
	t.Helper()
	engine, err := commandset.NewBoundEngineForSource(commandengine.SourceMessage, []commandset.BoundSurface{{Surface: c, ComponentRef: Type, ComponentType: Type}})
	if err != nil {
		t.Fatalf("NewBoundEngineForSource() error = %v", err)
	}
	return engine
}

func testRequest(threadID modeluuid.UUID) commandengine.Request {
	return commandengine.Request{Context: commandengine.Context{Source: commandengine.SourceMessage, ThreadID: threadID, Actor: coremodel.Actor{ID: "tester", Label: "tester", Roles: []simplerbac.Role{simplerbac.RoleAgent}}}}
}

func componentUpdateRequest(threadID modeluuid.UUID) component.UpdateRequest {
	return component.UpdateRequest{ThreadID: threadID}
}
