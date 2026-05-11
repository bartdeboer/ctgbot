package runtimeimage

import (
	"reflect"
	"runtime"
	"testing"
)

func TestDockerBuildArgsIncludeConfiguredDockerfile(t *testing.T) {
	t.Parallel()

	got := dockerBuildArgs(Target{
		Image:      "ctgbot-codex:gpu",
		Dockerfile: "cuda.Dockerfile",
	}, true, nil)
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
