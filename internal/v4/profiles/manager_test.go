package profiles

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/go-clistate"
)

func TestResolveDefaultsToDockerUnderStateRoot(t *testing.T) {
	root := t.TempDir()
	manager := New(root, nil)

	profile, err := manager.Resolve("work")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if profile.Runtime != "docker" {
		t.Fatalf("Runtime = %q, want docker", profile.Runtime)
	}
	if got, want := profile.Root, filepath.Join(root, ".ctgbot", "profiles", "work"); got != want {
		t.Fatalf("Root = %q, want %q", got, want)
	}
}

func TestResolveUsesConfiguredRuntimeAndHomePath(t *testing.T) {
	root := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	}()

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("NewCwd() error = %v", err)
	}
	manager := New(root, store)
	if err := manager.Set("personal", Settings{
		Runtime:  "local",
		HomePath: "profiles/personal-root",
	}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	profile, err := manager.Resolve("personal")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if profile.Runtime != "local" {
		t.Fatalf("Runtime = %q, want local", profile.Runtime)
	}
	if got, want := profile.Root, filepath.Join(root, "profiles", "personal-root"); got != want {
		t.Fatalf("Root = %q, want %q", got, want)
	}
}
