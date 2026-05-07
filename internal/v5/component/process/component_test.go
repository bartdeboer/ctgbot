package process

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
)

func TestProcessCommandDefinitionsDoNotExposeHostbridgeRoutes(t *testing.T) {
	component := New(nil)
	for _, definition := range component.CommandDefinitions() {
		for _, source := range definition.Sources {
			if source == commandengine.SourceHostbridge {
				t.Fatalf("definition %q unexpectedly exposes hostbridge source", definition.ID())
			}
		}
	}
}
