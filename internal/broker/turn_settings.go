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
	case "voice.language":
		r.settings.Voice.Language = cleanLanguageCode(value)
	case "voice.name":
		r.settings.Voice.Name = value
	case "voice.model":
		r.settings.Voice.Model = value
	default:
		return commandengine.Result{}, unknownTurnConfig(key)
	}
	return commandengine.Result{Text: fmt.Sprintf("turn config %s=%s", key, turnConfigValue(r.settings, key))}, nil
}

func (r *agentTurnRuntime) getTurnConfig(key string) (commandengine.Result, error) {
	if r == nil {
		return commandengine.Result{}, fmt.Errorf("missing turn runtime")
	}
	key = normalizeTurnConfigKey(key)
	if !knownTurnConfig(key) {
		return commandengine.Result{}, unknownTurnConfig(key)
	}
	return commandengine.Result{Text: fmt.Sprintf("turn config %s=%s", key, turnConfigValue(r.settings, key))}, nil
}

func (r *agentTurnRuntime) listTurnConfig() (commandengine.Result, error) {
	if r == nil {
		return commandengine.Result{}, fmt.Errorf("missing turn runtime")
	}
	return commandengine.Result{Text: formatTurnConfig(r.settings)}, nil
}

func normalizeTurnConfigKey(key string) string {
	return strings.TrimSpace(strings.ToLower(key))
}

func knownTurnConfig(key string) bool {
	switch key {
	case "voice.language", "voice.name", "voice.model":
		return true
	default:
		return false
	}
}

func turnConfigValue(settings turnSettings, key string) string {
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

func formatTurnConfig(settings turnSettings) string {
	return strings.Join([]string{
		"turn config voice.language=" + settings.Voice.Language,
		"turn config voice.name=" + settings.Voice.Name,
		"turn config voice.model=" + settings.Voice.Model,
	}, "\n")
}

func unknownTurnConfig(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("missing turn config key")
	}
	return fmt.Errorf("unknown turn config %q", key)
}
