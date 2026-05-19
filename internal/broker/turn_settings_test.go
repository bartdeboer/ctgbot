package broker

import (
	"context"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

func TestTurnCommandExecutorUpdatesCurrentTurnOnly(t *testing.T) {
	turn := &agentTurnRuntime{}
	executor := turnCommandExecutor{turn: turn}

	if _, err := executor.Execute(context.Background(), commandengine.Request{Command: schemacommands.TurnSet{Key: "voice.language", Value: "nl-NL"}}); err != nil {
		t.Fatalf("turn set language error = %v", err)
	}
	if _, err := executor.Execute(context.Background(), commandengine.Request{Command: schemacommands.TurnSet{Key: "voice.name", Value: "F3"}}); err != nil {
		t.Fatalf("turn set voice error = %v", err)
	}
	if got, want := turn.settings.Voice.Language, "nl"; got != want {
		t.Fatalf("voice language = %q, want %q", got, want)
	}
	if got, want := turn.settings.Voice.Name, "F3"; got != want {
		t.Fatalf("voice name = %q, want %q", got, want)
	}

	result, err := executor.Execute(context.Background(), commandengine.Request{Command: schemacommands.TurnGet{}})
	if err != nil {
		t.Fatalf("turn get error = %v", err)
	}
	if !strings.Contains(result.Text, "turn voice.language=nl") || !strings.Contains(result.Text, "turn voice.name=F3") {
		t.Fatalf("turn get text = %q, want current settings", result.Text)
	}

	if _, err := executor.Execute(context.Background(), commandengine.Request{Command: schemacommands.TurnClear{Key: "voice"}}); err != nil {
		t.Fatalf("turn clear error = %v", err)
	}
	if turn.settings.Voice != (turnVoiceSettings{}) {
		t.Fatalf("voice settings = %#v, want cleared", turn.settings.Voice)
	}
}

func TestTurnCommandExecutorRejectsUnknownSettings(t *testing.T) {
	turn := &agentTurnRuntime{}
	executor := turnCommandExecutor{turn: turn}

	_, err := executor.Execute(context.Background(), commandengine.Request{Command: schemacommands.TurnSet{Key: "unknown", Value: "x"}})
	if err == nil || !strings.Contains(err.Error(), "unknown turn setting") {
		t.Fatalf("turn set unknown error = %v, want unknown setting", err)
	}
}

func TestSpeechRequestForTurnUsesExplicitVoiceSettingsFirst(t *testing.T) {
	threadID := modeluuid.New()
	req := speechRequestForTurn("This response is in English.", threadID, turnOptions{SpeechLanguage: "nl"}, turnSettings{Voice: turnVoiceSettings{
		Language: "NL-nl",
		Name:     "F5",
		Model:    "supertonic3",
	}})

	if req.ThreadID != threadID {
		t.Fatalf("thread id = %s, want %s", req.ThreadID, threadID)
	}
	if req.Language != "nl" || req.Voice != "F5" || req.Model != "supertonic3" {
		t.Fatalf("speech request = %#v, want explicit turn voice settings", req)
	}
}

func TestSpeechRequestForTurnFallsBackToInputVoiceLanguage(t *testing.T) {
	req := speechRequestForTurn("Oké, dat klinkt goed.", modeluuid.New(), turnOptions{SpeechLanguage: "nl"}, turnSettings{})
	if req.Language != "nl" {
		t.Fatalf("language = %q, want nl", req.Language)
	}
}
