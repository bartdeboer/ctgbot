package indexing

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func TestResolveScopeDefaultsSchedulerToAll(t *testing.T) {
	scope, err := resolveScope(commandengine.Context{Source: commandengine.SourceScheduler}, scopeFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if !scope.All || !scope.ChatID.IsNull() || !scope.ThreadID.IsNull() {
		t.Fatalf("scope = %#v, want all", scope)
	}
}

func TestResolveScopeSchedulerHonorsExplicitThread(t *testing.T) {
	threadID := modeluuid.New()
	scope, err := resolveScope(commandengine.Context{Source: commandengine.SourceScheduler}, scopeFlags{Thread: threadID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if scope.All || scope.ThreadID != threadID {
		t.Fatalf("scope = %#v, want explicit thread", scope)
	}
}
