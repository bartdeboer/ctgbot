package app

import (
	"testing"

	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
)

func TestDedupeRuntimeImageTargetsOrdersDependencies(t *testing.T) {
	targets, err := dedupeRuntimeImageTargets([]runtimeimage.Target{
		{Name: "app", Image: "ctgbot-app:latest", Dockerfile: "app.Dockerfile", Uses: &runtimeimage.Target{Name: "base", Image: "ctgbot-base:latest", Dockerfile: "base.Dockerfile", Uses: &runtimeimage.Target{Name: "toolchain", Image: "ctgbot-toolchain:latest", Dockerfile: "toolchain.Dockerfile"}}},
	})
	if err != nil {
		t.Fatalf("dedupeRuntimeImageTargets() error = %v", err)
	}
	if len(targets) != 3 || targets[0].Name != "toolchain" || targets[1].Name != "base" || targets[2].Name != "app" {
		t.Fatalf("targets = %#v, want toolchain before base before app", targets)
	}
}
