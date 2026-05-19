package thread

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configsurface/conformtest"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

func TestSurfaceConformance(t *testing.T) {
	thread := &coremodel.Thread{}
	conformtest.Assert(t, NewSurface(thread), commandengine.Request{Context: commandengine.Context{Source: commandengine.SourceHostbridge}}, conformtest.Case{
		WritableKey:      VoiceLanguage,
		WritableValue:    "NL-nl",
		ExpectedSetValue: "nl",
		ExpectedUnset:    "",
	})
}
