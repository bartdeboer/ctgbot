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

func TestImageContextsConflictAndMixedOrder(t *testing.T) {
	for _, contexts := range [][2]string{{"/one", "/two"}, {"", "/one"}} {
		_, err := dedupeRuntimeImageTargets([]runtimeimage.Target{
			{Image: "same", Dockerfile: "Dockerfile", Context: contexts[0]},
			{Image: "same", Dockerfile: "Dockerfile", Context: contexts[1]},
		})
		if err == nil {
			t.Fatalf("accepted conflicting contexts %v", contexts)
		}
	}
	targets, err := dedupeRuntimeImageTargets([]runtimeimage.Target{
		{Name: "external", Image: "app", Context: "/host/app", Uses: &runtimeimage.Target{Name: "embedded", Image: "base", Uses: &runtimeimage.Target{Name: "external-base", Image: "tools", Context: "/host/tools"}}},
		{Name: "duplicate", Image: "tools", Context: "/host/tools/."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 3 || targets[0].Image != "tools" || targets[1].Image != "base" || targets[2].Image != "app" {
		t.Fatalf("targets=%+v", targets)
	}
	if targets[1].Context != "" || targets[2].Context != "/host/app" {
		t.Fatal("contexts inherited or lost")
	}
}
