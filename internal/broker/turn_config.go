package broker

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

const (
	turnConfigInputVoice        = "input.voice"
	turnConfigInputLanguage     = "input.language"
	turnConfigVoiceOutput       = "voice.output"
	turnConfigVoiceLanguage     = "voice.language"
	turnConfigVoiceName         = "voice.name"
	turnConfigVoiceModel        = "voice.model"
	turnConfigVoiceDeviceTarget = "voice.device-target"
)

type turnCommandExecutor struct {
	turn *agentTurnRuntime
	next commandengine.CommandExecutor
}

func (e turnCommandExecutor) Execute(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	switch cmd := req.Command.(type) {
	case schemacommands.TurnConfigSet:
		return e.turn.setTurnConfig(cmd.Key, cmd.Value)
	case schemacommands.TurnConfigGet:
		return e.turn.getTurnConfig(cmd.Key)
	case schemacommands.TurnConfigList:
		return e.turn.listTurnConfig()
	default:
		if e.next == nil {
			return commandengine.Result{}, fmt.Errorf("missing command executor")
		}
		return e.next.Execute(ctx, req)
	}
}

func (r *agentTurnRuntime) setTurnConfig(key string, value string) (commandengine.Result, error) {
	if r == nil {
		return commandengine.Result{}, fmt.Errorf("missing turn runtime")
	}
	key = normalizeTurnConfigKey(key)
	value = strings.TrimSpace(value)
	switch key {
	case turnConfigVoiceOutput:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return commandengine.Result{}, fmt.Errorf("turn config %s expects true or false", key)
		}
		r.voiceOutput = parsed
	case turnConfigVoiceLanguage:
		r.voiceLanguage = cleanLanguageCode(value)
	case turnConfigVoiceName:
		r.voiceName = value
	case turnConfigVoiceModel:
		r.voiceModel = value
	case turnConfigVoiceDeviceTarget:
		r.voiceDeviceTarget = value
	case turnConfigInputVoice, turnConfigInputLanguage:
		return commandengine.Result{}, fmt.Errorf("turn config %s is read-only", key)
	default:
		return commandengine.Result{}, unknownTurnConfig(key)
	}
	return commandengine.Result{Text: fmt.Sprintf("turn config %s=%s", key, r.turnConfigValue(key))}, nil
}

func (r *agentTurnRuntime) getTurnConfig(key string) (commandengine.Result, error) {
	if r == nil {
		return commandengine.Result{}, fmt.Errorf("missing turn runtime")
	}
	key = normalizeTurnConfigKey(key)
	if !knownTurnConfig(key) {
		return commandengine.Result{}, unknownTurnConfig(key)
	}
	return commandengine.Result{Text: fmt.Sprintf("turn config %s=%s", key, r.turnConfigValue(key))}, nil
}

func (r *agentTurnRuntime) listTurnConfig() (commandengine.Result, error) {
	if r == nil {
		return commandengine.Result{}, fmt.Errorf("missing turn runtime")
	}
	return commandengine.Result{Text: strings.Join([]string{
		"turn config " + turnConfigInputVoice + "=" + r.turnConfigValue(turnConfigInputVoice),
		"turn config " + turnConfigInputLanguage + "=" + r.turnConfigValue(turnConfigInputLanguage),
		"turn config " + turnConfigVoiceOutput + "=" + r.turnConfigValue(turnConfigVoiceOutput),
		"turn config " + turnConfigVoiceLanguage + "=" + r.turnConfigValue(turnConfigVoiceLanguage),
		"turn config " + turnConfigVoiceName + "=" + r.turnConfigValue(turnConfigVoiceName),
		"turn config " + turnConfigVoiceModel + "=" + r.turnConfigValue(turnConfigVoiceModel),
		"turn config " + turnConfigVoiceDeviceTarget + "=" + r.turnConfigValue(turnConfigVoiceDeviceTarget),
	}, "\n")}, nil
}

func (r *agentTurnRuntime) turnConfigValue(key string) string {
	if r == nil {
		return ""
	}
	switch key {
	case turnConfigInputVoice:
		return strconv.FormatBool(r.voiceInput)
	case turnConfigInputLanguage:
		return r.detectedInputLanguage
	case turnConfigVoiceOutput:
		return strconv.FormatBool(r.voiceOutput)
	case turnConfigVoiceLanguage:
		return r.voiceLanguage
	case turnConfigVoiceName:
		return r.voiceName
	case turnConfigVoiceModel:
		return r.voiceModel
	case turnConfigVoiceDeviceTarget:
		return r.voiceDeviceTarget
	default:
		return ""
	}
}

func (r *agentTurnRuntime) applyThreadVoiceConfig(thread coremodel.Thread) {
	if r == nil {
		return
	}
	if thread.VoiceOutput || (r.voiceInput && thread.VoiceReplyToVoiceInput) {
		r.voiceOutput = true
	}
	if language := cleanLanguageCode(thread.VoiceLanguage); language != "" {
		r.voiceLanguage = language
	}
	if voice := strings.TrimSpace(thread.VoiceName); voice != "" {
		r.voiceName = voice
	}
	if model := strings.TrimSpace(thread.VoiceModel); model != "" {
		r.voiceModel = model
	}
	if target := strings.TrimSpace(thread.VoiceDeviceTarget); target != "" {
		r.voiceDeviceTarget = target
	}
}

func normalizeTurnConfigKey(key string) string {
	return strings.ReplaceAll(strings.TrimSpace(strings.ToLower(key)), "_", "-")
}

func knownTurnConfig(key string) bool {
	switch key {
	case turnConfigInputVoice,
		turnConfigInputLanguage,
		turnConfigVoiceOutput,
		turnConfigVoiceLanguage,
		turnConfigVoiceName,
		turnConfigVoiceModel,
		turnConfigVoiceDeviceTarget:
		return true
	default:
		return false
	}
}

func unknownTurnConfig(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("missing turn config key")
	}
	return fmt.Errorf("unknown turn config %q", key)
}
