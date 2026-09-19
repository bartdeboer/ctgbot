package cmdsurface

import (
	"bytes"
	"encoding/gob"
	"reflect"
	"testing"

	gmailv2component "github.com/bartdeboer/ctgbot/internal/component/gmailv2"
	llamacppcomponent "github.com/bartdeboer/ctgbot/internal/component/llamacpp"
	"github.com/bartdeboer/go-clir"
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

func TestCommandRefBoundSurfacesSupportsGmailV2(t *testing.T) {
	bound := CommandRefBoundSurfaces("gmailv2/work")
	if len(bound) != 1 {
		t.Fatalf("len(CommandRefBoundSurfaces) = %d, want 1", len(bound))
	}
	if got, want := bound[0].ComponentType, gmailv2component.Type; got != want {
		t.Fatalf("ComponentType = %q, want %q", got, want)
	}
	if got, want := bound[0].ComponentRef, "gmailv2/work"; got != want {
		t.Fatalf("ComponentRef = %q, want %q", got, want)
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
	if bound := BoundSurfaces("unknown/work"); len(bound) != 0 {
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
		if prefix == "turn" {
			return
		}
	}
	t.Fatalf("GlobalDirectPrefixes() = %#v, want turn", prefixes)
}

func TestThreadInfoGobForAllProviders(t *testing.T) {
	RegisterGobTypes(gob.Register)
	for _, ref := range []string{"copilot/work", "codex/work", "claude/work"} {
		found := false
		for _, def := range Resolve(ref).Surface.CommandDefinitions() {
			if def.Pattern != "thread info" {
				continue
			}
			found = true
			value, err := def.Build(&clir.Request{})
			if err != nil {
				t.Fatal(err)
			}
			var wire bytes.Buffer
			if err := gob.NewEncoder(&wire).Encode(&value); err != nil {
				t.Fatal(err)
			}
			var decoded any
			if err := gob.NewDecoder(&wire).Decode(&decoded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(value, decoded) {
				t.Fatal("wire changed", ref)
			}
		}
		if !found {
			t.Fatal("missing thread info", ref)
		}
	}
}

func TestGlobalThreadInfoGob(t *testing.T) {
	RegisterGobTypes(gob.Register)
	for _, surface := range GlobalSurfaces() {
		for _, def := range surface.CommandDefinitions() {
			if def.Pattern != "thread info" {
				continue
			}
			value, err := def.Build(&clir.Request{})
			if err != nil {
				t.Fatal(err)
			}
			var wire bytes.Buffer
			if err := gob.NewEncoder(&wire).Encode(&value); err != nil {
				t.Fatal(err)
			}
			var decoded any
			if err := gob.NewDecoder(&wire).Decode(&decoded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(value, decoded) {
				t.Fatal("wire changed")
			}
			return
		}
	}
	t.Fatal("missing global thread info")
}
