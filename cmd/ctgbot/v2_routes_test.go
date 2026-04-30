package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v2runtime "github.com/bartdeboer/ctgbot/internal/v2/runtime"
	"github.com/bartdeboer/go-clir"
)

func TestV2RunCreatesConfigAndDatabase(t *testing.T) {
	withTempCwd(t, func(root string) {
		router := clir.New()
		registerV2Routes(router)

		output := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"run"}); err != nil {
				t.Fatalf("run: %v", err)
			}
		})
		if !strings.Contains(output, "ctgbot v2 runtime initialized") {
			t.Fatalf("unexpected output: %q", output)
		}
		assertFileExists(t, filepath.Join(root, ".ctgbot", "configv2.json"))
		assertFileExists(t, filepath.Join(root, ".ctgbot", "ctgbotv2.db"))
	})
}

func TestV2ComponentAuthCreatesProfileAndRepositoryRows(t *testing.T) {
	withTempCwd(t, func(root string) {
		router := clir.New()
		registerV2Routes(router)

		output := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"component", "auth", "codex", "personal", "--prepare-only"}); err != nil {
				t.Fatalf("component auth: %v", err)
			}
		})
		if !strings.Contains(output, "component profile ready") || !strings.Contains(output, "container_path: /profile") || !strings.Contains(output, "auth: prepare only") {
			t.Fatalf("unexpected output: %q", output)
		}
		assertDirExists(t, filepath.Join(root, ".ctgbot", "profiles", "codex", "personal"))

		store, err := v2runtime.OpenStorage(context.Background(), filepath.Join(root, ".ctgbot", "ctgbotv2.db"))
		if err != nil {
			t.Fatalf("open v2 storage: %v", err)
		}
		component, err := store.Components().GetByType(context.Background(), "codex")
		if err != nil {
			t.Fatalf("get component: %v", err)
		}
		if component == nil || !component.Enabled {
			t.Fatalf("unexpected component: %#v", component)
		}
		profile, err := store.ComponentProfiles().Get(context.Background(), "codex", "personal")
		if err != nil {
			t.Fatalf("get profile: %v", err)
		}
		if profile == nil || !profile.Enabled {
			t.Fatalf("unexpected profile: %#v", profile)
		}
	})
}

func TestV2ComponentAuthReportsMissingAuthenticator(t *testing.T) {
	withTempCwd(t, func(root string) {
		router := clir.New()
		registerV2Routes(router)

		output := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"component", "auth", "gmail", "work"}); err != nil {
				t.Fatalf("component auth gmail: %v", err)
			}
		})
		if !strings.Contains(output, "auth: not implemented yet") {
			t.Fatalf("unexpected output: %q", output)
		}
		assertDirExists(t, filepath.Join(root, ".ctgbot", "profiles", "gmail", "work"))
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

func assertFileExists(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("expected file %s, got directory", path)
	}
}

func assertDirExists(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected directory %s: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("expected directory %s", path)
	}
}
