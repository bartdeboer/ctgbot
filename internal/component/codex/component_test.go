package codex

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/component"
)

func TestComponentCapabilities(t *testing.T) {
	codex := New()
	registry := component.NewRegistry(codex)

	if codex.Type() != ComponentType {
		t.Fatalf("Type() = %q, want %q", codex.Type(), ComponentType)
	}
	if got := len(component.Capabilities[component.ProfileOwner](registry)); got != 1 {
		t.Fatalf("profile owner capabilities len = %d, want 1", got)
	}
}

func TestManagedFiles(t *testing.T) {
	files := New().ManagedFiles()
	if len(files) != 2 {
		t.Fatalf("managed files len = %d, want 2", len(files))
	}
	if files[0].RelativePath != "auth.json" || !files[0].Required || !files[0].Sensitive {
		t.Fatalf("unexpected auth file: %#v", files[0])
	}
	if files[1].RelativePath != "config.toml" || files[1].Required || files[1].Sensitive {
		t.Fatalf("unexpected config file: %#v", files[1])
	}
}
