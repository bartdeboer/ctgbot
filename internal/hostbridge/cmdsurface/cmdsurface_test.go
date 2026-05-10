package cmdsurface

import (
	"testing"

	llamacppcomponent "github.com/bartdeboer/ctgbot/internal/component/llamacpp"
)

func TestResolveFallsBackToCodexForInvalidRef(t *testing.T) {
	resolved := Resolve("///")
	if got, want := resolved.ComponentType, "codex"; got != want {
		t.Fatalf("ComponentType = %q, want %q", got, want)
	}
	if got, want := resolved.ComponentRef, "codex"; got != want {
		t.Fatalf("ComponentRef = %q, want %q", got, want)
	}
	if !resolved.Supported {
		t.Fatal("Supported = false, want true")
	}
}

func TestBoundSurfacesSupportsKnownTypes(t *testing.T) {
	bound := BoundSurfaces("llamacpp/default")
	if len(bound) != 1 {
		t.Fatalf("len(BoundSurfaces) = %d, want 1", len(bound))
	}
	if got, want := bound[0].ComponentType, llamacppcomponent.Type; got != want {
		t.Fatalf("ComponentType = %q, want %q", got, want)
	}
	if got, want := bound[0].ComponentRef, "llamacpp/default"; got != want {
		t.Fatalf("ComponentRef = %q, want %q", got, want)
	}
}

func TestBoundSurfacesIgnoresUnsupportedTypes(t *testing.T) {
	if bound := BoundSurfaces("gmail/work"); len(bound) != 0 {
		t.Fatalf("len(BoundSurfaces) = %d, want 0", len(bound))
	}
}

func TestDirectPrefixesIncludeTypeAndRef(t *testing.T) {
	prefixes := DirectPrefixes("llamacpp/default")
	if len(prefixes) != 2 {
		t.Fatalf("len(DirectPrefixes) = %d, want 2", len(prefixes))
	}
	if prefixes[0] != "llamacpp" || prefixes[1] != "llamacpp/default" {
		t.Fatalf("DirectPrefixes = %#v, want [llamacpp llamacpp/default]", prefixes)
	}
}

func TestGlobalDirectPrefixesIncludeStatus(t *testing.T) {
	prefixes := GlobalDirectPrefixes()
	for _, prefix := range prefixes {
		if prefix == "status" {
			return
		}
	}
	t.Fatalf("GlobalDirectPrefixes() = %#v, want status", prefixes)
}
