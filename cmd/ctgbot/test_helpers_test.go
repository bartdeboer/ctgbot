package main

import (
	"os"
	"testing"
)

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
