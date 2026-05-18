package supertonic

import (
	"testing"

	"github.com/bartdeboer/go-clir"
)

func TestCommandDefinitions(t *testing.T) {
	definitions := (&Component{}).CommandDefinitions()
	if len(definitions) != 1 {
		t.Fatalf("definitions = %#v", definitions)
	}
	if got, want := definitions[0].CanonicalPattern(), "speak <text>"; got != want {
		t.Fatalf("pattern = %q, want %q", got, want)
	}
	if err := definitions[0].Validate(); err != nil {
		t.Fatalf("definition invalid: %v", err)
	}
}

func TestBuildSpeakCommand(t *testing.T) {
	built, err := buildSpeakCommand(&clir.Request{
		Params: map[string]string{"text": "Hello"},
		Extra:  []string{"--model", "supertonic3-f1", "--voice", "F1", "--language", "nl"},
	})
	if err != nil {
		t.Fatalf("buildSpeakCommand() error = %v", err)
	}
	cmd := built.(speakCommand)
	if cmd.Text != "Hello" || cmd.Model != "supertonic3-f1" || cmd.Voice != "F1" || cmd.Language != "nl" {
		t.Fatalf("cmd = %#v", cmd)
	}
}
