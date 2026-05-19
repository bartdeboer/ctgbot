package broker

import (
	"context"
	"fmt"
	"strings"

	turnconfig "github.com/bartdeboer/ctgbot/internal/app/config/turn"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

const (
	turnConfigInputVoice        = turnconfig.InputVoice
	turnConfigInputLanguage     = turnconfig.InputLanguage
	turnConfigVoiceOutput       = turnconfig.VoiceOutput
	turnConfigVoiceLanguage     = turnconfig.VoiceLanguage
	turnConfigVoiceName         = turnconfig.VoiceName
	turnConfigVoiceModel        = turnconfig.VoiceModel
	turnConfigVoiceDeviceTarget = turnconfig.VoiceDeviceTarget
)

type turnCommandExecutor struct {
	turn *agentTurnRuntime
	next commandengine.CommandExecutor
}

func (e turnCommandExecutor) Execute(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	switch cmd := req.Command.(type) {
	case schemacommands.TurnConfigSet:
		return e.turn.setTurnConfig(ctx, req, cmd.Key, cmd.Value)
	case schemacommands.TurnConfigUnset:
		return e.turn.unsetTurnConfig(ctx, req, cmd.Key)
	case schemacommands.TurnConfigGet:
		return e.turn.getTurnConfig(ctx, req, cmd.Key)
	case schemacommands.TurnConfigList:
		return e.turn.listTurnConfig(ctx, req)
	default:
		if e.next == nil {
			return commandengine.Result{}, fmt.Errorf("missing command executor")
		}
		return e.next.Execute(ctx, req)
	}
}

func (r *agentTurnRuntime) setTurnConfig(ctx context.Context, req commandengine.Request, key string, value string) (commandengine.Result, error) {
	values := r.turnConfigValues()
	surface := turnconfig.NewSurface(&values)
	key = turnconfig.NormalizeKey(key)
	if err := surface.ConfigSet(ctx, req, key, value); err != nil {
		return commandengine.Result{}, err
	}
	r.applyTurnConfigValues(values)
	updated, err := surface.ConfigGet(ctx, req, key)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("turn config %s=%s", key, updated)}, nil
}

func (r *agentTurnRuntime) unsetTurnConfig(ctx context.Context, req commandengine.Request, key string) (commandengine.Result, error) {
	values := r.turnConfigValues()
	surface := turnconfig.NewSurface(&values)
	key = turnconfig.NormalizeKey(key)
	if err := surface.ConfigUnset(ctx, req, key); err != nil {
		return commandengine.Result{}, err
	}
	r.applyTurnConfigValues(values)
	updated, err := surface.ConfigGet(ctx, req, key)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("turn config %s=%s", key, updated)}, nil
}

func (r *agentTurnRuntime) getTurnConfig(ctx context.Context, req commandengine.Request, key string) (commandengine.Result, error) {
	values := r.turnConfigValues()
	surface := turnconfig.NewSurface(&values)
	key = turnconfig.NormalizeKey(key)
	field, ok := turnconfig.Schema().Field(key)
	if !ok {
		return commandengine.Result{}, turnconfig.UnknownKey(key)
	}
	value, err := surface.ConfigGet(ctx, req, field.Key)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: configsurface.FormatGet(field, value)}, nil
}

func (r *agentTurnRuntime) listTurnConfig(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	values := r.turnConfigValues()
	surface := turnconfig.NewSurface(&values)
	return commandengine.Result{Text: configsurface.FormatList(ctx, req, surface, turnconfig.Schema())}, nil
}

func (r *agentTurnRuntime) turnConfigValues() turnconfig.Values {
	if r == nil {
		return turnconfig.Values{}
	}
	return turnconfig.Values{
		InputVoice:        r.voiceInput,
		InputLanguage:     r.detectedInputLanguage,
		VoiceOutput:       r.voiceOutput,
		VoiceLanguage:     r.voiceLanguage,
		VoiceName:         r.voiceName,
		VoiceModel:        r.voiceModel,
		VoiceDeviceTarget: r.voiceDeviceTarget,
	}
}

func (r *agentTurnRuntime) applyTurnConfigValues(values turnconfig.Values) {
	if r == nil {
		return
	}
	r.voiceOutput = values.VoiceOutput
	r.voiceLanguage = values.VoiceLanguage
	r.voiceName = values.VoiceName
	r.voiceModel = values.VoiceModel
	r.voiceDeviceTarget = values.VoiceDeviceTarget
}

func (r *agentTurnRuntime) turnConfigValue(key string) string {
	value, _ := turnconfig.Value(r.turnConfigValues(), key)
	return value
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
