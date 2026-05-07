package routers_test

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/schema/routers"
)

func TestSourceDefinitionsExcludeCLIOnlyScaffold(t *testing.T) {
	for _, tc := range []struct {
		name        string
		definitions []commandengine.Definition
	}{
		{name: "message", definitions: routers.MessageDefinitions()},
		{name: "hostbridge", definitions: routers.HostbridgeDefinitions()},
	} {
		if hasRoute(tc.definitions, "config hostbridge scaffold <alias>") {
			t.Fatalf("%s definitions include CLI-only hostbridge scaffold route", tc.name)
		}
	}
}

func hasRoute(definitions []commandengine.Definition, pattern string) bool {
	for _, definition := range definitions {
		for _, route := range definition.Routes() {
			if commandengine.NormalizePattern(route.Pattern) == commandengine.NormalizePattern(pattern) {
				return true
			}
		}
	}
	return false
}
