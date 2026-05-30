package indexing

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func TestResolveScopeDefaultsToAll(t *testing.T) {
	scope, err := resolveScope(commandengine.Context{}, scopeFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if !scope.All || !scope.ChatID.IsNull() || !scope.ThreadID.IsNull() {
		t.Fatalf("scope = %#v, want all", scope)
	}
}

func TestResolveScopeHonorsExplicitThread(t *testing.T) {
	threadID := modeluuid.New()
	scope, err := resolveScope(commandengine.Context{}, scopeFlags{Thread: threadID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if scope.All || scope.ThreadID != threadID {
		t.Fatalf("scope = %#v, want explicit thread", scope)
	}
}
