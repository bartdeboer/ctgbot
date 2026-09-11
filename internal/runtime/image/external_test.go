package runtimeimage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func externalFixture(t *testing.T) Target {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "recipes"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "recipes", "Dockerfile"), []byte("FROM scratch\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return Target{Image: "example:test", Context: root, Dockerfile: "recipes/Dockerfile"}
}

func TestExternalBuildPassesDirectoryWithoutTar(t *testing.T) {
	target := externalFixture(t)
	bin := t.TempDir()
	argsFile, inputFile, cwdFile := filepath.Join(bin, "args"), filepath.Join(bin, "input"), filepath.Join(bin, "cwd")
	t.Setenv("ARGS_FILE", argsFile)
	t.Setenv("INPUT_FILE", inputFile)
	t.Setenv("CWD_FILE", cwdFile)
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ARGS_FILE\"\npwd > \"$CWD_FILE\"\ncat > \"$INPUT_FILE\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := (&Builder{}).BuildTarget(context.Background(), target, true); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(target.Context)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(args), root+"\n") || !strings.Contains(string(args), "-f\nrecipes/Dockerfile\n") || !strings.Contains(string(args), "--no-cache\n") {
		t.Fatalf("args=%s", args)
	}
	stdin, err := os.ReadFile(inputFile)
	if err != nil || len(stdin) != 0 {
		t.Fatalf("stdin bytes=%d err=%v", len(stdin), err)
	}
	cwd, err := os.ReadFile(cwdFile)
	if err != nil || strings.TrimSpace(string(cwd)) != root {
		t.Fatalf("cwd=%s err=%v", cwd, err)
	}
	for _, label := range []string{LabelGitCommit, LabelHostbridge, LabelVersion} {
		if strings.Contains(string(args), label+"=") {
			t.Fatalf("misleading label %s", label)
		}
	}
}

func TestExternalInvalidPathsBeforeDocker(t *testing.T) {
	base := externalFixture(t)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("FROM scratch"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(base.Context, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ context, file string }{
		{"relative", "Dockerfile"},
		{filepath.Join(base.Context, "missing"), "Dockerfile"},
		{outside, "Dockerfile"},
		{base.Context, "../outside"},
		{base.Context, outside},
		{base.Context, "escape"},
		{base.Context, "missing"},
		{base.Context, "recipes"},
		{base.Context, "recipes\\Dockerfile"},
	} {
		target := base
		target.Context = tc.context
		target.Dockerfile = tc.file
		// No docker exists in PATH: error must originate in validation.
		t.Setenv("PATH", t.TempDir())
		err := (&Builder{}).BuildTarget(t.Context(), target, false)
		if err == nil || strings.Contains(err.Error(), "docker build") {
			t.Fatalf("context=%s file=%s err=%v", tc.context, tc.file, err)
		}
	}
}

func TestTargetContextIsNeverInherited(t *testing.T) {
	target := externalFixture(t)
	target.Uses = &Target{Image: "embedded:base", Dockerfile: "codex.Dockerfile"}
	got, err := normalizeTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	if got.Uses.Context != "" {
		t.Fatal("inherited external context")
	}
	parent := Target{Image: "embedded:parent", Uses: &target}
	got, err = normalizeTarget(parent)
	if err != nil {
		t.Fatal(err)
	}
	if got.Context != "" || got.Uses.Context == "" {
		t.Fatal("lost independent context")
	}
}
