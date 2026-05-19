package broker

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

type turnSettings struct {
	Voice turnVoiceSettings
}

type turnVoiceSettings struct {
	Language string
	Name     string
	Model    string
}

type turnCommandExecutor struct {
	turn *agentTurnRuntime
	next commandengine.CommandExecutor
}

func (e turnCommandExecutor) Execute(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	switch cmd := req.Command.(type) {
	case schemacommands.TurnSet:
		return e.turn.setTurnSetting(cmd.Key, cmd.Value)
	case schemacommands.TurnGet:
		return e.turn.getTurnSetting(cmd.Key)
	case schemacommands.TurnClear:
		return e.turn.clearTurnSetting(cmd.Key)
	default:
		if e.next == nil {
			return commandengine.Result{}, fmt.Errorf("missing command executor")
		}
		return e.next.Execute(ctx, req)
	}
}

func (r *agentTurnRuntime) setTurnSetting(key string, value string) (commandengine.Result, error) {
	if r == nil {
		return commandengine.Result{}, fmt.Errorf("missing turn runtime")
	}
	key = normalizeTurnSettingKey(key)
	value = strings.TrimSpace(value)
	switch key {
	case "voice.language":
		r.settings.Voice.Language = cleanLanguageCode(value)
	case "voice.name":
		r.settings.Voice.Name = value
	case "voice.model":
		r.settings.Voice.Model = value
	default:
		return commandengine.Result{}, unknownTurnSetting(key)
	}
	return commandengine.Result{Text: fmt.Sprintf("turn %s=%s", key, turnSettingValue(r.settings, key))}, nil
}

func (r *agentTurnRuntime) getTurnSetting(key string) (commandengine.Result, error) {
	if r == nil {
		return commandengine.Result{}, fmt.Errorf("missing turn runtime")
	}
	key = normalizeTurnSettingKey(key)
	if key == "" {
		return commandengine.Result{Text: formatTurnSettings(r.settings)}, nil
	}
	if !knownTurnSetting(key) {
		return commandengine.Result{}, unknownTurnSetting(key)
	}
	return commandengine.Result{Text: fmt.Sprintf("turn %s=%s", key, turnSettingValue(r.settings, key))}, nil
}

func (r *agentTurnRuntime) clearTurnSetting(key string) (commandengine.Result, error) {
	if r == nil {
		return commandengine.Result{}, fmt.Errorf("missing turn runtime")
	}
	key = normalizeTurnSettingKey(key)
	switch key {
	case "voice":
		r.settings.Voice = turnVoiceSettings{}
	case "voice.language":
		r.settings.Voice.Language = ""
	case "voice.name":
		r.settings.Voice.Name = ""
	case "voice.model":
		r.settings.Voice.Model = ""
	default:
		return commandengine.Result{}, unknownTurnSetting(key)
	}
	return commandengine.Result{Text: "turn cleared: " + key}, nil
}

func normalizeTurnSettingKey(key string) string {
	return strings.TrimSpace(strings.ToLower(key))
}

func knownTurnSetting(key string) bool {
	switch key {
	case "voice.language", "voice.name", "voice.model":
		return true
	default:
		return false
	}
}

func turnSettingValue(settings turnSettings, key string) string {
	switch key {
	case "voice.language":
		return settings.Voice.Language
	case "voice.name":
		return settings.Voice.Name
	case "voice.model":
		return settings.Voice.Model
	default:
		return ""
	}
}

func formatTurnSettings(settings turnSettings) string {
	return strings.Join([]string{
		"turn voice.language=" + settings.Voice.Language,
		"turn voice.name=" + settings.Voice.Name,
		"turn voice.model=" + settings.Voice.Model,
	}, "\n")
}

func unknownTurnSetting(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("missing turn setting key")
	}
	return fmt.Errorf("unknown turn setting %q", key)
}
