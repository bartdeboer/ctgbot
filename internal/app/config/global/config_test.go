package global

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configengine"
	"github.com/bartdeboer/ctgbot/internal/configsurface/conformtest"
	configschema "github.com/bartdeboer/ctgbot/internal/schema/config"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clistate"
)

func TestSurfaceConformance(t *testing.T) {
	cfg := newTestConfig(t)
	registry, err := configschema.Registry(cfg)
	if err != nil {
		t.Fatalf("config registry: %v", err)
	}
	surface := New(configengine.New(registry))
	conformtest.Assert(t, surface, commandengine.Request{Context: commandengine.Context{
		Source: commandengine.SourceHostbridge,
		Actor:  commandengine.Actor{ID: "root", Roles: []simplerbac.Role{simplerbac.RoleRoot}},
	}}, conformtest.Case{
		WritableKey:      "docker.image",
		WritableValue:    "ctgbot:test",
		ExpectedSetValue: "ctgbot:test",
		ReadOnlyKey:      "codex.login-callback-port",
	})
}

func newTestConfig(t *testing.T) *appstate.Config {
	t.Helper()
	root := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new cwd store: %v", err)
	}
	return appstate.New(filepath.Join(root, ".ctgbot"), store)
}
