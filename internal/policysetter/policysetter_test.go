package policysetter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/configsetters"
	"github.com/bartdeboer/go-clistate"
)

func newTestRegistry(t *testing.T) (*Registry, *configsetters.ConfigSetters, Context) {
	t.Helper()
	root := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("WriteFile go.mod: %v", err)
	}
	local, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("NewCwd: %v", err)
	}
	global, err := clistate.NewGlobal("ctgbot", "config")
	if err != nil {
		t.Fatalf("NewGlobal: %v", err)
	}
	state, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), local)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	entry, err := state.EnsureProviderChat("telegram", "123", "Test Chat")
	if err != nil {
		t.Fatalf("EnsureProviderChat: %v", err)
	}
	cfg := configsetters.NewConfigSetters(state, local, global)
	return NewDefaultRegistry(cfg), cfg, Context{ChatID: entry.ID, UserID: 1, Elevation: ElevationChat}
}

func TestRegistryListFiltersByElevation(t *testing.T) {
	registry, _, ctx := newTestRegistry(t)
	if got := len(registry.List(ctx)); got != 0 {
		t.Fatalf("chat visible setters = %d, want 0", got)
	}
	ctx.Elevation = ElevationElevated
	if got := len(registry.List(ctx)); got < 4 {
		t.Fatalf("elevated visible setters = %d, want at least 4", got)
	}
}

func TestRegistryFindByName(t *testing.T) {
	registry, _, _ := newTestRegistry(t)
	if _, ok := registry.Find("session.timeout_min"); !ok {
		t.Fatalf("expected session.timeout_min setter")
	}
	if _, ok := registry.Find("missing"); ok {
		t.Fatalf("unexpected setter for missing")
	}
}

func TestChatCannotSetElevatedProperty(t *testing.T) {
	registry, _, ctx := newTestRegistry(t)
	setter, ok := registry.Find("session.timeout_min")
	if !ok {
		t.Fatalf("missing setter")
	}
	if _, err := setter.Set(ctx, "90s"); err == nil {
		t.Fatalf("expected elevation restriction error")
	}
}

func TestElevatedCanSetSessionTimeout(t *testing.T) {
	registry, cfg, ctx := newTestRegistry(t)
	ctx.Elevation = ElevationElevated
	setter, ok := registry.Find("session.timeout_min")
	if !ok {
		t.Fatalf("missing setter")
	}
	got, err := setter.Set(ctx, "90s")
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got != "90s" {
		t.Fatalf("Set returned %q, want 90s", got)
	}
	if stored := cfg.Local.GetString("session.timeout_min", ""); stored != "90s" {
		t.Fatalf("stored timeout = %q, want 90s", stored)
	}
}

func TestElevatedCanSetProcessTools(t *testing.T) {
	registry, cfg, ctx := newTestRegistry(t)
	ctx.Elevation = ElevationElevated
	setter, ok := registry.Find("chat.process_tools_enabled")
	if !ok {
		t.Fatalf("missing setter")
	}
	got, err := setter.Set(ctx, "true")
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got != "true" {
		t.Fatalf("Set returned %q, want true", got)
	}
	if !cfg.State.ChatProcessToolsEnabledByID(ctx.ChatID) {
		t.Fatalf("expected process tools enabled")
	}
}
