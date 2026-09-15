package main

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/buildassets"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/component/copilot"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/hostbridge/cmdsurface"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	dockerruntime "github.com/bartdeboer/ctgbot/internal/runtime/docker"
	systempkg "github.com/bartdeboer/ctgbot/internal/system"
)

func TestCopilotRealRegistrationDiscoveryAndImageWiring(t *testing.T) {
	registry, err := newRuntimeRegistry(&systempkg.System{Logger: log.New(io.Discard, "", 0)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"copilot", "codex", "claude"} {
		if !registry.Has(kind) {
			t.Fatal("missing registration", kind)
		}
	}
	root := t.TempDir()
	profile := runtimepkg.Profile{Path: filepath.Join(root, "profile")}
	if err := os.MkdirAll(profile.Path, 0700); err != nil {
		t.Fatal(err)
	}
	// The real Docker factory may bind configuration, but nil sandbox/bridge
	// dependencies make any accidental runtime startup fail. No Docker commands.
	factory := dockerruntime.New(root, filepath.Join(root, "components"), nil, nil)
	reg := coremodel.Component{ID: modeluuid.New(), Type: "copilot", Name: "review", ProfilePath: profile.Path}
	loaded, err := registry.Build(context.Background(), reg, factory, profile, repository.NewMemory())
	if err != nil {
		t.Fatal(err)
	}
	provider, ok := loaded.Component.(*copilot.Component)
	if !ok {
		t.Fatalf("%T", loaded.Component)
	}
	if err := provider.AuthStatus(t.Context(), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	surface := cmdsurface.Resolve("copilot/review")
	if !surface.Supported || surface.ComponentType != "copilot" {
		t.Fatal(surface)
	}
	for _, def := range surface.Surface.CommandDefinitions() {
		if def.Pattern == "goal" || def.Pattern == "compact" {
			t.Fatal(def.Pattern)
		}
	}
	if !cmdsurface.Resolve("codex/codex").Supported || !cmdsurface.Resolve("claude/claude").Supported {
		t.Fatal("existing discovery changed")
	}
	targets, err := loaded.Component.(component.RuntimeImageProvider).RuntimeImageTargets(t.Context())
	if err != nil || len(targets) != 1 {
		t.Fatal(targets, err)
	}
	target := targets[0]
	if target.Image != "ctgbot-copilot:latest" || target.Dockerfile != "copilot.Dockerfile" || target.Uses == nil || target.Uses.Image != "ctgbot-copilot-base:latest" || target.Uses.Uses == nil || target.Uses.Uses.Image != "ctgbot-go-node-python-base:latest" {
		t.Fatalf("wrong image chain %+v", target)
	}
	// Source inclusion in the *real* embedded-context selection; generating the
	// tar/build is a separate authorized deployment step, not done by these tests.
	selected := false
	for _, spec := range buildassets.SelectedFiles() {
		if spec.Source == "docker" && spec.Target == "." {
			selected = true
		}
	}
	if !selected {
		t.Fatal("Docker recipes excluded from embedded context")
	}
	for _, file := range []string{target.Dockerfile, target.Uses.Dockerfile, target.Uses.Uses.Dockerfile} {
		if _, err := os.Stat(filepath.Join("..", "..", "docker", file)); err != nil {
			t.Fatal(file, err)
		}
	}
	recipe, err := os.ReadFile(filepath.Join("..", "..", "docker", "copilot.base.Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"COPILOT_VERSION=" + copilot.CLIVersion, "sha256sum --check", "arm64", "amd64", "COPILOT_AUTO_UPDATE=false"} {
		if !strings.Contains(string(recipe), needle) {
			t.Fatal("recipe missing", needle)
		}
	}
	entries, err := os.ReadDir(profile.Path)
	if err != nil || len(entries) != 0 {
		t.Fatal("discovery modified profile", entries, err)
	}
}
