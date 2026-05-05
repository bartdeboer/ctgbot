package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/go-clistate"
)

func TestLoadWorkspacesReturnsConfiguredEntriesOnly(t *testing.T) {
	root := t.TempDir()
	workspaces, err := LoadWorkspaces(root, nil)
	if err != nil {
		t.Fatalf("LoadWorkspaces() error = %v", err)
	}
	if len(workspaces) != 0 {
		t.Fatalf("workspaces = %#v, want none", workspaces)
	}
}

func TestSaveWorkspaceUsesConfiguredPath(t *testing.T) {
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
	workspace, err := SaveWorkspace(root, store, "personal", "workspaces/personal-root")
	if err != nil {
		t.Fatalf("SaveWorkspace() error = %v", err)
	}
	if got, want := workspace.Path, filepath.Join(root, "workspaces", "personal-root"); got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestConfiguredWorkspacesReadsCurrentKey(t *testing.T) {
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
	current := map[string]WorkspaceSettings{
		"work": {Path: "workspaces/work-root"},
	}
	if err := store.PersistStruct(workspaceConfigKey, current); err != nil {
		t.Fatalf("PersistStruct(%q) error = %v", workspaceConfigKey, err)
	}

	configured := ConfiguredWorkspaces(store)
	if got, want := configured["work"].Path, "workspaces/work-root"; got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}
