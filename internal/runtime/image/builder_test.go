package runtimeimage

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	appstate "github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/go-clistate"
)

func TestDockerBuildArgsIncludeConfiguredDockerfile(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWD) })

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	if err := cfg.Docker().SetImage("ctgbot-codex:gpu"); err != nil {
		t.Fatalf("set image: %v", err)
	}
	if err := cfg.Docker().SetDockerfile("cuda.Dockerfile"); err != nil {
		t.Fatalf("set dockerfile: %v", err)
	}

	got := dockerBuildArgs(DefaultTarget(cfg), true, nil)
	want := []string{
		"build",
		"-f", "cuda.Dockerfile",
		"-t", "ctgbot-codex:gpu",
		"--build-arg", "TARGETARCH=" + runtime.GOARCH,
		"--no-cache",
		"-",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dockerBuildArgs() = %#v, want %#v", got, want)
	}
}

func TestDockerBuildArgsIncludeSortedLabels(t *testing.T) {
	t.Parallel()

	got := dockerBuildArgs(Target{
		Image:      "ctgbot-codex:test",
		Dockerfile: "Dockerfile",
	}, false, map[string]string{
		"z.label": "last",
		"a.label": "first",
	})
	want := []string{
		"build",
		"-f", "Dockerfile",
		"-t", "ctgbot-codex:test",
		"--build-arg", "TARGETARCH=" + runtime.GOARCH,
		"--label", "a.label=first",
		"--label", "z.label=last",
		"-",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dockerBuildArgs() = %#v, want %#v", got, want)
	}
}
