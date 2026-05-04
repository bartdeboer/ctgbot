package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/go-clistate"
)

func TestLoadProfilesDefaultsToDockerUnderStateRoot(t *testing.T) {
	root := t.TempDir()
	profiles, err := LoadProfiles(root, nil)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	profile := profiles["default"]
	if profile.Runtime != "docker" {
		t.Fatalf("Runtime = %q, want docker", profile.Runtime)
	}
	if got, want := profile.Root, filepath.Join(root, ".ctgbot", "profiles", "default"); got != want {
		t.Fatalf("Root = %q, want %q", got, want)
	}
}

func TestSaveProfileUsesConfiguredRuntimeAndHomePath(t *testing.T) {
	root := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.MkdirAll(".ctgbot", 0o755); err != nil {
		t.Fatalf("MkdirAll(.ctgbot) error = %v", err)
	}

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("NewCwd() error = %v", err)
	}
	profile, err := SaveProfile(root, store, "personal", "local", "profiles/personal-root")
	if err != nil {
		t.Fatalf("SaveProfile() error = %v", err)
	}
	if profile.Runtime != "local" {
		t.Fatalf("Runtime = %q, want local", profile.Runtime)
	}
	if got, want := profile.Root, filepath.Join(root, "profiles", "personal-root"); got != want {
		t.Fatalf("Root = %q, want %q", got, want)
	}
}

func TestConfiguredProfilesFallsBackToLegacyKey(t *testing.T) {
	root := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.MkdirAll(".ctgbot", 0o755); err != nil {
		t.Fatalf("MkdirAll(.ctgbot) error = %v", err)
	}

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("NewCwd() error = %v", err)
	}
	legacy := map[string]ProfileSettings{
		"work": {Runtime: "docker", HomePath: "profiles/work-root"},
	}
	if err := store.PersistStruct(legacyProfileConfigKey, legacy); err != nil {
		t.Fatalf("PersistStruct(%q) error = %v", legacyProfileConfigKey, err)
	}

	configured := ConfiguredProfiles(store)
	if got, want := configured["work"].Runtime, "docker"; got != want {
		t.Fatalf("Runtime = %q, want %q", got, want)
	}
	if got, want := configured["work"].HomePath, "profiles/work-root"; got != want {
		t.Fatalf("HomePath = %q, want %q", got, want)
	}
}
