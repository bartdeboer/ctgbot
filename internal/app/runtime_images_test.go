package app

import (
	"testing"

	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
)

func TestDedupeRuntimeImageTargetsOrdersDependencies(t *testing.T) {
	targets, err := dedupeRuntimeImageTargets([]runtimeimage.Target{
		{Name: "app", Image: "ctgbot-app:latest", Dockerfile: "app.Dockerfile", Uses: &runtimeimage.Target{Name: "base", Image: "ctgbot-base:latest", Dockerfile: "base.Dockerfile"}},
	})
	if err != nil {
		t.Fatalf("dedupeRuntimeImageTargets() error = %v", err)
	}
	if len(targets) != 2 || targets[0].Name != "base" || targets[1].Name != "app" {
		t.Fatalf("targets = %#v, want base before app", targets)
	}
}
