// Package turn owns ephemeral per-turn configuration.
package turn

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
)

const (
	InputVoice        = "input.voice"
	InputLanguage     = "input.language"
	VoiceOutput       = "voice.output"
	VoiceLanguage     = "voice.language"
	VoiceName         = "voice.name"
	VoiceModel        = "voice.model"
	VoiceDeviceTarget = "voice.device-target"
)

type Values struct {
	InputVoice        bool
	InputLanguage     string
	VoiceOutput       bool
	VoiceLanguage     string
	VoiceName         string
	VoiceModel        string
	VoiceDeviceTarget string
}

type Surface struct {
	Values *Values
}

func NewSurface(values *Values) *Surface {
	return &Surface{Values: values}
}

func Schema() configsurface.ConfigSchema {
	return configsurface.ConfigSchema{Fields: []configsurface.FieldSchema{
		{Key: InputVoice, Help: "Whether this turn started from voice input", Type: configsurface.FieldTypeBool, Writable: false, Default: "false", Options: []string{"true", "false"}},
		{Key: InputLanguage, Help: "Detected language of the input message", Type: configsurface.FieldTypeString, Writable: false},
		{Key: VoiceOutput, Help: "Include generated voice output for this turn", Type: configsurface.FieldTypeBool, Writable: true, Default: "false", Options: []string{"true", "false"}},
		{Key: VoiceLanguage, Help: "Voice synthesis language for this turn", Type: configsurface.FieldTypeString, Writable: true},
		{Key: VoiceName, Help: "Voice name/style for this turn", Type: configsurface.FieldTypeEnum, Writable: true, Options: []string{"F1", "F2", "F3", "F4", "F5", "M1", "M2", "M3", "M4", "M5"}},
		{Key: VoiceModel, Help: "Voice synthesis model for this turn", Type: configsurface.FieldTypeString, Writable: true},
		{Key: VoiceDeviceTarget, Help: "Optional target for future voice output devices", Type: configsurface.FieldTypeString, Writable: true},
	}}
}

func (s *Surface) ConfigSchema(ctx context.Context, req commandengine.Request) (configsurface.ConfigSchema, error) {
	_, _ = ctx, req
	return Schema(), nil
}

func (s *Surface) ConfigGet(ctx context.Context, req commandengine.Request, key string) (string, error) {
	_, _ = ctx, req
	if s == nil || s.Values == nil {
		return "", fmt.Errorf("missing turn config")
	}
	return Value(*s.Values, key)
}

func (s *Surface) ConfigSet(ctx context.Context, req commandengine.Request, key string, value string) error {
	_, _ = ctx, req
	if s == nil || s.Values == nil {
		return fmt.Errorf("missing turn config")
	}
	_, err := Set(s.Values, key, value)
	return err
}

func (s *Surface) ConfigUnset(ctx context.Context, req commandengine.Request, key string) error {
	_, _ = ctx, req
	if s == nil || s.Values == nil {
		return fmt.Errorf("missing turn config")
	}
	_, err := Unset(s.Values, key)
	return err
}

func Value(values Values, key string) (string, error) {
	switch NormalizeKey(key) {
	case InputVoice:
		return strconv.FormatBool(values.InputVoice), nil
	case InputLanguage:
		return strings.TrimSpace(values.InputLanguage), nil
	case VoiceOutput:
		return strconv.FormatBool(values.VoiceOutput), nil
	case VoiceLanguage:
		return strings.TrimSpace(values.VoiceLanguage), nil
	case VoiceName:
		return strings.TrimSpace(values.VoiceName), nil
	case VoiceModel:
		return strings.TrimSpace(values.VoiceModel), nil
	case VoiceDeviceTarget:
		return strings.TrimSpace(values.VoiceDeviceTarget), nil
	default:
		return "", UnknownKey(key)
	}
}

func Set(values *Values, key string, value string) (string, error) {
	if values == nil {
		return "", fmt.Errorf("missing turn config")
	}
	key = NormalizeKey(key)
	value = strings.TrimSpace(value)
	switch key {
	case VoiceOutput:
		parsed, err := parseBool(key, value)
		if err != nil {
			return "", err
		}
		values.VoiceOutput = parsed
	case VoiceLanguage:
		values.VoiceLanguage = cleanLanguage(value)
	case VoiceName:
		values.VoiceName = value
	case VoiceModel:
		values.VoiceModel = value
	case VoiceDeviceTarget:
		values.VoiceDeviceTarget = value
	case InputVoice, InputLanguage:
		return "", fmt.Errorf("turn config %s is read-only", key)
	default:
		return "", UnknownKey(key)
	}
	return Value(*values, key)
}

func Unset(values *Values, key string) (string, error) {
	if values == nil {
		return "", fmt.Errorf("missing turn config")
	}
	key = NormalizeKey(key)
	switch key {
	case VoiceOutput:
		values.VoiceOutput = false
	case VoiceLanguage:
		values.VoiceLanguage = ""
	case VoiceName:
		values.VoiceName = ""
	case VoiceModel:
		values.VoiceModel = ""
	case VoiceDeviceTarget:
		values.VoiceDeviceTarget = ""
	case InputVoice, InputLanguage:
		return "", fmt.Errorf("turn config %s is read-only", key)
	default:
		return "", UnknownKey(key)
	}
	return Value(*values, key)
}

func NormalizeKey(key string) string {
	return strings.ReplaceAll(strings.TrimSpace(strings.ToLower(key)), "_", "-")
}

func KnownKey(key string) bool {
	_, err := Value(Values{}, key)
	return err == nil
}

func UnknownKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("missing turn config key")
	}
	return fmt.Errorf("unknown turn config %q", NormalizeKey(key))
}

func parseBool(key string, value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on", "enabled":
		return true, nil
	case "false", "0", "no", "off", "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("turn config %s expects true or false", key)
	}
}

func cleanLanguage(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if idx := strings.IndexAny(value, "-_"); idx >= 0 {
		value = value[:idx]
	}
	return value
}
