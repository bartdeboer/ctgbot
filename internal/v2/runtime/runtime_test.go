package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/go-clistate"
)

func TestOpenResolvesRuntimePaths(t *testing.T) {
	withTempCwd(t, func(root string) {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("get cwd: %v", err)
		}
		rt, err := Open(context.Background(), Options{DBPath: "custom/ctgbot.db"})
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		if rt.StateRoot != filepath.Join(cwd, ".ctgbot") {
			t.Fatalf("StateRoot = %q, want %q", rt.StateRoot, filepath.Join(cwd, ".ctgbot"))
		}
		if rt.ConfigPath != filepath.Join(cwd, ".ctgbot", "config.json") {
			t.Fatalf("ConfigPath = %q", rt.ConfigPath)
		}
		if rt.DBPath != filepath.Join(cwd, "custom", "ctgbot.db") {
			t.Fatalf("DBPath = %q", rt.DBPath)
		}
		if rt.Profiles.ContainerPath() != "/profile" {
			t.Fatalf("profile container path = %q", rt.Profiles.ContainerPath())
		}
	})
}

func TestResolveTelegramTokenPrefersFlagThenEnv(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "from-env")

	if got := ResolveTelegramToken(" from-flag ", nil); got != "from-flag" {
		t.Fatalf("flag token = %q", got)
	}
	if got := ResolveTelegramToken("", nil); got != "from-env" {
		t.Fatalf("env token = %q", got)
	}
}

func TestResolveCodexProfilePrefersFlag(t *testing.T) {
	if got := ResolveCodexProfile(" v2test ", nil); got != "v2test" {
		t.Fatalf("profile = %q, want v2test", got)
	}
}

func TestResolveOperatorTelegramUserIDs(t *testing.T) {
	withTempCwd(t, func(root string) {
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("new store: %v", err)
		}
		if err := clistate.PersistTyped(store, "operators", OperatorConfig{TelegramUserIDs: []int64{13145044}}); err != nil {
			t.Fatalf("persist operators: %v", err)
		}

		ids := ResolveOperatorTelegramUserIDs(store)
		if len(ids) != 1 || ids[0] != 13145044 {
			t.Fatalf("operator ids = %#v", ids)
		}
	})
}

func TestWireBrokerRequiresTelegramToken(t *testing.T) {
	withTempCwd(t, func(root string) {
		rt, err := Open(context.Background(), Options{})
		if err != nil {
			t.Fatalf("open runtime: %v", err)
		}
		if _, err := WireBroker(context.Background(), rt, BrokerOptions{}); err == nil {
			t.Fatal("expected missing telegram token error")
		}
	})
}

func withTempCwd(t *testing.T, fn func(root string)) {
	t.Helper()

	root := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	defer func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	fn(root)
}
