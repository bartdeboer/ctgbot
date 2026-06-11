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

func TestRuntimePortsConfigNormalizesJSONList(t *testing.T) {
	thread := &coremodel.Thread{}
	value, err := Set(thread, RuntimePorts, `["127.0.0.1:18423:8080","127.0.0.1:18424:3000"]`)
	if err != nil {
		t.Fatalf("Set(runtime.ports) error = %v", err)
	}
	if got, want := value, `["127.0.0.1:18423:8080","127.0.0.1:18424:3000"]`; got != want {
		t.Fatalf("Set value = %q, want %q", got, want)
	}
	if got, want := RuntimePortsValue(*thread), []string{"127.0.0.1:18423:8080", "127.0.0.1:18424:3000"}; !equalStrings(got, want) {
		t.Fatalf("RuntimePortsValue = %#v, want %#v", got, want)
	}
}

func TestRuntimePortsConfigAcceptsCommaList(t *testing.T) {
	thread := &coremodel.Thread{}
	if _, err := Set(thread, RuntimePorts, "127.0.0.1:18423:8080,127.0.0.1:18424:3000"); err != nil {
		t.Fatalf("Set(runtime.ports comma list) error = %v", err)
	}
	if got, want := Value(*thread, RuntimePorts); got != `["127.0.0.1:18423:8080","127.0.0.1:18424:3000"]` {
		t.Fatalf("Value(runtime.ports) = %q, want %q", got, want)
	}
}

func equalStrings(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
