package profilemanager

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHostPathResolvesNamedComponentProfile(t *testing.T) {
	root := t.TempDir()
	manager := New(root)

	got, err := manager.HostPath("codex", "personal")
	if err != nil {
		t.Fatalf("HostPath() error = %v", err)
	}
	want := filepath.Join(root, ".ctgbot", "profiles", "codex", "personal")
	if got != want {
		t.Fatalf("HostPath() = %q, want %q", got, want)
	}
}

func TestContainerPathUsesUniversalProfileMount(t *testing.T) {
	manager := New(t.TempDir())

	if got := manager.ContainerPath(); got != "/profile" {
		t.Fatalf("ContainerPath() = %q, want /profile", got)
	}
}

func TestEnsureCreatesProfileDirectory(t *testing.T) {
	root := t.TempDir()
	manager := New(root)

	path, err := manager.Ensure("gmail", "work")
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("profile dir missing: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("profile path is not a directory: %s", path)
	}
}

func TestMountReturnsHostAndContainerPaths(t *testing.T) {
	root := t.TempDir()
	manager := New(root)

	mount, err := manager.Mount("codex", "personal")
	if err != nil {
		t.Fatalf("Mount() error = %v", err)
	}
	wantHost := filepath.Join(root, ".ctgbot", "profiles", "codex", "personal")
	if mount.HostPath != wantHost || mount.ContainerPath != "/profile" {
		t.Fatalf("unexpected mount: %#v", mount)
	}
}

func TestHostPathRejectsUnsafeNames(t *testing.T) {
	manager := New(t.TempDir())

	for _, tc := range []struct {
		component string
		profile   string
	}{
		{component: "", profile: "work"},
		{component: "gmail", profile: ""},
		{component: "../gmail", profile: "work"},
		{component: "gmail", profile: "../work"},
		{component: "gmail/work", profile: "work"},
		{component: "gmail", profile: "work/personal"},
		{component: ".", profile: "work"},
		{component: "gmail", profile: ".."},
	} {
		if _, err := manager.HostPath(tc.component, tc.profile); err == nil {
			t.Fatalf("HostPath(%q, %q) succeeded, want error", tc.component, tc.profile)
		}
	}
}

func TestHostPathRejectsMissingRoot(t *testing.T) {
	if _, err := New("").HostPath("gmail", "work"); err == nil {
		t.Fatal("HostPath() succeeded with missing root, want error")
	}
}
