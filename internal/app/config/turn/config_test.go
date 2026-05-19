package turn

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configsurface/conformtest"
)

func TestSurfaceConformance(t *testing.T) {
	values := &Values{InputVoice: true, InputLanguage: "nl"}
	conformtest.Assert(t, NewSurface(values), commandengine.Request{Context: commandengine.Context{Source: commandengine.SourceHostbridge}}, conformtest.Case{
		WritableKey:      VoiceLanguage,
		WritableValue:    "NL-nl",
		ExpectedSetValue: "nl",
		ExpectedUnset:    "",
		ReadOnlyKey:      InputVoice,
	})
}
