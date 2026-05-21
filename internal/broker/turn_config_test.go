package broker

import (
	"context"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
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
		"input.voice=true",
		"input.language=nl",
		"voice.output=true",
		"voice.language=nl",
		"voice.name=F3",
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

	unset, err := executor.Execute(context.Background(), commandengine.Request{Command: schemacommands.TurnConfigUnset{Key: "voice.name"}})
	if err != nil {
		t.Fatalf("turn config unset error = %v", err)
	}
	if got, want := strings.TrimSpace(unset.Text), "voice.name="; got != want {
		t.Fatalf("turn config unset text = %q, want %q", got, want)
	}
	if turn.voiceName != "" {
		t.Fatalf("voice name = %q, want unset", turn.voiceName)
	}
}

func TestTurnCommandExecutorPreservesActiveComponents(t *testing.T) {
	next := commandengine.NewEngine(nil, nil).WithActiveComponentRefs([]string{"codex", "gmailv2/personal"})
	executor := turnCommandExecutor{next: next}

	got := executor.ActiveComponents()
	if strings.Join(got, ",") != "codex,gmailv2/personal" {
		t.Fatalf("ActiveComponents() = %#v", got)
	}
}

func TestTurnConfigListUsesModelRegistryMetadata(t *testing.T) {
	turn := &agentTurnRuntime{
		runtime: &ChatRuntime{Components: []*component.Loaded{
			{Component: fakeModelRegistry{}},
		}},
	}
	executor := turnCommandExecutor{turn: turn}

	result, err := executor.Execute(context.Background(), commandengine.Request{Command: schemacommands.TurnConfigList{}})
	if err != nil {
		t.Fatalf("turn config list error = %v", err)
	}
	for _, want := range []string{
		"voice.model=supertonic",
		"options: supertonic, kyutai",
		"voice.name=",
		"options: F1, F5",
		"default: F5",
	} {
		if !strings.Contains(result.Text, want) {
			t.Fatalf("turn config list text = %q, want %q", result.Text, want)
		}
	}
}

func TestTurnInfoShowsInputMetadataAndFiles(t *testing.T) {
	turn := &agentTurnRuntime{
		voiceInput:            true,
		detectedInputLanguage: "nl",
		inputFiles: []turnInputFile{
			{Path: "/workspace/turn-input/abc/voice-input.ogg", Kind: "voice", Filename: "voice-input.ogg", ContentType: "audio/ogg", Temporary: true},
		},
	}
	executor := turnCommandExecutor{turn: turn}

	result, err := executor.Execute(context.Background(), commandengine.Request{Command: schemacommands.TurnInfo{}})
	if err != nil {
		t.Fatalf("turn info error = %v", err)
	}
	for _, want := range []string{
		"voice: true",
		"detected_language: nl",
		"/workspace/turn-input/abc/voice-input.ogg",
		"temporary=true",
		"hostbridge turn config list",
	} {
		if !strings.Contains(result.Text, want) {
			t.Fatalf("turn info text = %q, want %q", result.Text, want)
		}
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

type fakeModelRegistry struct{}

func (fakeModelRegistry) Type() string { return "model" }

func (fakeModelRegistry) ListModels(ctx context.Context) ([]component.Model, error) {
	_ = ctx
	return []component.Model{
		{Name: "supertonic", Mode: component.ModelModeTTS},
		{Name: "kyutai", Mode: component.ModelModeTTS},
		{Name: "qwen", Mode: component.ModelModeCompletion},
	}, nil
}

func (fakeModelRegistry) GetModel(ctx context.Context, name string) (component.Model, error) {
	_ = ctx
	return component.Model{Name: name, Mode: component.ModelModeTTS}, nil
}

func (fakeModelRegistry) InstallModel(ctx context.Context, req component.ModelInstallRequest) (component.Model, error) {
	_ = ctx
	return req.Model, nil
}

func (fakeModelRegistry) RegisterModel(ctx context.Context, req component.ModelInstallRequest) (component.Model, error) {
	_ = ctx
	return req.Model, nil
}

func (fakeModelRegistry) DefaultModel(ctx context.Context) (string, error) {
	_ = ctx
	return "qwen", nil
}

func (fakeModelRegistry) DefaultModelForMode(ctx context.Context, mode component.ModelMode) (string, error) {
	_ = ctx
	if mode == component.ModelModeTTS {
		return "supertonic", nil
	}
	return "", nil
}

func (fakeModelRegistry) ModelCard(ctx context.Context, name string) (string, error) {
	_, _ = ctx, name
	return "", nil
}

func (fakeModelRegistry) ModelConfigSchema(ctx context.Context, name string) (configsurface.ConfigSchema, error) {
	_, _ = ctx, name
	return configsurface.ConfigSchema{Fields: []configsurface.FieldSchema{
		{Key: turnConfigVoiceName, Help: "Voice style", Type: configsurface.FieldTypeEnum, Default: "F5", Options: []string{"F1", "F5"}},
	}}, nil
}
