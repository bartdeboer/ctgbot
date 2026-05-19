package broker

import (
	"context"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

func TestTurnCommandExecutorUpdatesCurrentTurnOnly(t *testing.T) {
	turn := &agentTurnRuntime{voiceInput: true, detectedInputLanguage: "nl"}
	executor := turnCommandExecutor{turn: turn}

	if _, err := executor.Execute(context.Background(), commandengine.Request{Command: schemacommands.TurnConfigSet{Key: "voice.output", Value: "true"}}); err != nil {
		t.Fatalf("turn config set voice output error = %v", err)
	}
	if _, err := executor.Execute(context.Background(), commandengine.Request{Command: schemacommands.TurnConfigSet{Key: "voice.language", Value: "nl-NL"}}); err != nil {
		t.Fatalf("turn config set language error = %v", err)
	}
	if _, err := executor.Execute(context.Background(), commandengine.Request{Command: schemacommands.TurnConfigSet{Key: "voice.name", Value: "F3"}}); err != nil {
		t.Fatalf("turn config set voice error = %v", err)
	}
	if !turn.voiceOutput {
		t.Fatal("voice output = false, want true")
	}
	if got, want := turn.voiceLanguage, "nl"; got != want {
		t.Fatalf("voice language = %q, want %q", got, want)
	}
	if got, want := turn.voiceName, "F3"; got != want {
		t.Fatalf("voice name = %q, want %q", got, want)
	}

	result, err := executor.Execute(context.Background(), commandengine.Request{Command: schemacommands.TurnConfigList{}})
	if err != nil {
		t.Fatalf("turn config list error = %v", err)
	}
	for _, want := range []string{
		"turn config input.voice=true",
		"turn config input.language=nl",
		"turn config voice.output=true",
		"turn config voice.language=nl",
		"turn config voice.name=F3",
	} {
		if !strings.Contains(result.Text, want) {
			t.Fatalf("turn config list text = %q, want %q", result.Text, want)
		}
	}

	if _, err := executor.Execute(context.Background(), commandengine.Request{Command: schemacommands.TurnConfigSet{Key: "voice.name", Value: "F5"}}); err != nil {
		t.Fatalf("turn config set overwrite error = %v", err)
	}
	if got, want := turn.voiceName, "F5"; got != want {
		t.Fatalf("voice name = %q, want overwritten value %q", got, want)
	}
}

func TestTurnCommandExecutorRejectsUnknownAndReadOnlySettings(t *testing.T) {
	turn := &agentTurnRuntime{}
	executor := turnCommandExecutor{turn: turn}

	_, err := executor.Execute(context.Background(), commandengine.Request{Command: schemacommands.TurnConfigSet{Key: "unknown", Value: "x"}})
	if err == nil || !strings.Contains(err.Error(), "unknown turn config") {
		t.Fatalf("turn config set unknown error = %v, want unknown setting", err)
	}

	_, err = executor.Execute(context.Background(), commandengine.Request{Command: schemacommands.TurnConfigSet{Key: "input.voice", Value: "false"}})
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("turn config set input.voice error = %v, want read-only", err)
	}
}

func TestApplyThreadVoiceConfigEnablesVoiceRepliesToVoiceInput(t *testing.T) {
	turn := &agentTurnRuntime{voiceInput: true}
	turn.applyThreadVoiceConfig(coremodel.Thread{
		VoiceReplyToVoiceInput: true,
		VoiceLanguage:          "nl-NL",
		VoiceName:              "F5",
		VoiceModel:             "supertonic3",
	})

	if !turn.voiceOutput {
		t.Fatal("voice output = false, want enabled for voice input")
	}
	if turn.voiceLanguage != "nl" || turn.voiceName != "F5" || turn.voiceModel != "supertonic3" {
		t.Fatalf("turn voice config = language:%q name:%q model:%q", turn.voiceLanguage, turn.voiceName, turn.voiceModel)
	}
}

func TestApplyThreadVoiceConfigDoesNotEnableReplyToTextInput(t *testing.T) {
	turn := &agentTurnRuntime{}
	turn.applyThreadVoiceConfig(coremodel.Thread{VoiceReplyToVoiceInput: true})
	if turn.voiceOutput {
		t.Fatal("voice output = true, want false for text input")
	}
}

func TestSpeechRequestForTurnUsesExplicitVoiceConfigFirst(t *testing.T) {
	threadID := modeluuid.New()
	turn := &agentTurnRuntime{
		thread:                coremodel.Thread{ID: threadID},
		detectedInputLanguage: "nl",
		voiceLanguage:         "NL-nl",
		voiceName:             "F5",
		voiceModel:            "supertonic3",
	}
	req := speechRequestForTurn("This response is in English.", turn)

	if req.ThreadID != threadID {
		t.Fatalf("thread id = %s, want %s", req.ThreadID, threadID)
	}
	if req.Language != "nl" || req.Voice != "F5" || req.Model != "supertonic3" {
		t.Fatalf("speech request = %#v, want explicit turn voice config", req)
	}
}

func TestSpeechRequestForTurnFallsBackToInputVoiceLanguage(t *testing.T) {
	req := speechRequestForTurn("Oké, dat klinkt goed.", &agentTurnRuntime{detectedInputLanguage: "nl"})
	if req.Language != "nl" {
		t.Fatalf("language = %q, want nl", req.Language)
	}
}
