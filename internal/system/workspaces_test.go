package system

import (
	"os"
	"path/filepath"
	"testing"

	hostbridgepolicy "github.com/bartdeboer/ctgbot/internal/hostbridgepolicy"
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

func TestSaveWorkspacePreservesHostbridgeAliases(t *testing.T) {
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
	if err := store.PersistStruct(workspaceConfigKey, map[string]WorkspaceSettings{
		"work": {
			Path: "workspaces/old-root",
			Hostbridge: WorkspaceHostbridgeSettings{
				Aliases: map[string]hostbridgepolicy.Alias{
					"echo-work": {Name: "/bin/echo", Args: []string{"work"}},
				},
			},
		},
	}); err != nil {
		t.Fatalf("PersistStruct(%q) error = %v", workspaceConfigKey, err)
	}

	workspace, err := SaveWorkspace(root, store, "work", "workspaces/new-root")
	if err != nil {
		t.Fatalf("SaveWorkspace() error = %v", err)
	}
	if got, want := workspace.Path, filepath.Join(root, "workspaces", "new-root"); got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
	if got := workspace.HostbridgeAliases["echo-work"].Name; got != "/bin/echo" {
		t.Fatalf("Aliases[echo-work].Name = %q, want /bin/echo", got)
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
		"work": {
			Path: "workspaces/work-root",
			Hostbridge: WorkspaceHostbridgeSettings{
				Aliases: map[string]hostbridgepolicy.Alias{
					"echo-work": {Name: "/bin/echo", Args: []string{"work"}},
				},
			},
		},
	}
	if err := store.PersistStruct(workspaceConfigKey, current); err != nil {
		t.Fatalf("PersistStruct(%q) error = %v", workspaceConfigKey, err)
	}

	configured := ConfiguredWorkspaces(store)
	if got, want := configured["work"].Path, "workspaces/work-root"; got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
	if got := configured["work"].Hostbridge.Aliases["echo-work"].Name; got != "/bin/echo" {
		t.Fatalf("Aliases[echo-work].Name = %q, want /bin/echo", got)
	}
}

func TestLoadWorkspacesReadsHostbridgeAliases(t *testing.T) {
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
		"work": {
			Path: "workspaces/work-root",
			Hostbridge: WorkspaceHostbridgeSettings{
				Aliases: map[string]hostbridgepolicy.Alias{
					"echo-work": {Name: "/bin/echo", Args: []string{"work"}},
				},
			},
		},
	}
	if err := store.PersistStruct(workspaceConfigKey, current); err != nil {
		t.Fatalf("PersistStruct(%q) error = %v", workspaceConfigKey, err)
	}

	workspaces, err := LoadWorkspaces(root, store)
	if err != nil {
		t.Fatalf("LoadWorkspaces() error = %v", err)
	}
	workspace := workspaces["work"]
	if got, want := workspace.Path, filepath.Join(root, "workspaces", "work-root"); got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
	command := workspace.HostbridgeAliases["echo-work"]
	if got, want := command.Name, "/bin/echo"; got != want {
		t.Fatalf("Aliases[echo-work].Name = %q, want %q", got, want)
	}
	if got, want := len(command.Args), 1; got != want {
		t.Fatalf("Aliases[echo-work].Args len = %d, want %d", got, want)
	}
}
