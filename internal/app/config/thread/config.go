// Package thread owns broker-managed persistent thread configuration.
package thread

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

const (
	VoiceReplyToVoiceInput = "voice.reply-to-voice-input"
	VoiceOutput            = "voice.output"
	VoiceLanguage          = "voice.language"
	VoiceName              = "voice.name"
	VoiceModel             = "voice.model"
	VoiceDeviceTarget      = "voice.device-target"
)

var keys = []string{
	VoiceReplyToVoiceInput,
	VoiceOutput,
	VoiceLanguage,
	VoiceName,
	VoiceModel,
	VoiceDeviceTarget,
}

// Surface exposes a thread row through the shared config surface contract.
// It does not own storage; callers decide when to load and persist the thread.
type Surface struct {
	Thread *coremodel.Thread
}

func NewSurface(thread *coremodel.Thread) *Surface {
	return &Surface{Thread: thread}
}

func Schema() configsurface.ConfigSchema {
	return configsurface.ConfigSchema{Fields: []configsurface.FieldSchema{
		{
			Key:      VoiceReplyToVoiceInput,
			Help:     "Reply with generated voice when the incoming message was voice input",
			Type:     configsurface.FieldTypeBool,
			Writable: true,
			Default:  "false",
			Options:  []string{"true", "false"},
		},
		{
			Key:      VoiceOutput,
			Help:     "Always include generated voice output for this thread",
			Type:     configsurface.FieldTypeBool,
			Writable: true,
			Default:  "false",
			Options:  []string{"true", "false"},
		},
		{
			Key:      VoiceLanguage,
			Help:     "Preferred voice synthesis language code",
			Type:     configsurface.FieldTypeString,
			Writable: true,
		},
		{
			Key:      VoiceName,
			Help:     "Preferred voice name/style",
			Type:     configsurface.FieldTypeEnum,
			Writable: true,
			Options:  []string{"F1", "F2", "F3", "F4", "F5", "M1", "M2", "M3", "M4", "M5"},
		},
		{
			Key:      VoiceModel,
			Help:     "Preferred voice synthesis model",
			Type:     configsurface.FieldTypeString,
			Writable: true,
		},
		{
			Key:      VoiceDeviceTarget,
			Help:     "Optional target for future voice output devices",
			Type:     configsurface.FieldTypeString,
			Writable: true,
		},
	}}
}

func Keys() []string {
	return append([]string{}, keys...)
}

func (s *Surface) ConfigSchema(ctx context.Context, req commandengine.Request) (configsurface.ConfigSchema, error) {
	_, _ = ctx, req
	return Schema(), nil
}

func (s *Surface) ConfigGet(ctx context.Context, req commandengine.Request, key string) (string, error) {
	_, _ = ctx, req
	if s == nil || s.Thread == nil {
		return "", fmt.Errorf("missing thread")
	}
	return Value(*s.Thread, key)
}

func (s *Surface) ConfigSet(ctx context.Context, req commandengine.Request, key string, value string) error {
	_, _ = ctx, req
	if s == nil || s.Thread == nil {
		return fmt.Errorf("missing thread")
	}
	_, err := Set(s.Thread, key, value)
	return err
}

func (s *Surface) ConfigUnset(ctx context.Context, req commandengine.Request, key string) error {
	_, _ = ctx, req
	if s == nil || s.Thread == nil {
		return fmt.Errorf("missing thread")
	}
	_, err := Unset(s.Thread, key)
	return err
}

func Value(thread coremodel.Thread, key string) (string, error) {
	switch NormalizeKey(key) {
	case VoiceReplyToVoiceInput:
		return strconv.FormatBool(thread.VoiceReplyToVoiceInput), nil
	case VoiceOutput:
		return strconv.FormatBool(thread.VoiceOutput), nil
	case VoiceLanguage:
		return strings.TrimSpace(thread.VoiceLanguage), nil
	case VoiceName:
		return strings.TrimSpace(thread.VoiceName), nil
	case VoiceModel:
		return strings.TrimSpace(thread.VoiceModel), nil
	case VoiceDeviceTarget:
		return strings.TrimSpace(thread.VoiceDeviceTarget), nil
	default:
		return "", UnknownKey(key)
	}
}

func Set(thread *coremodel.Thread, key string, value string) (string, error) {
	if thread == nil {
		return "", fmt.Errorf("missing thread")
	}
	key = NormalizeKey(key)
	value = strings.TrimSpace(value)
	switch key {
	case VoiceReplyToVoiceInput:
		parsed, err := parseBool(key, value)
		if err != nil {
			return "", err
		}
		thread.VoiceReplyToVoiceInput = parsed
	case VoiceOutput:
		parsed, err := parseBool(key, value)
		if err != nil {
			return "", err
		}
		thread.VoiceOutput = parsed
	case VoiceLanguage:
		thread.VoiceLanguage = cleanLanguage(value)
	case VoiceName:
		thread.VoiceName = value
	case VoiceModel:
		thread.VoiceModel = value
	case VoiceDeviceTarget:
		thread.VoiceDeviceTarget = value
	default:
		return "", UnknownKey(key)
	}
	return Value(*thread, key)
}

func Unset(thread *coremodel.Thread, key string) (string, error) {
	if thread == nil {
		return "", fmt.Errorf("missing thread")
	}
	key = NormalizeKey(key)
	switch key {
	case VoiceReplyToVoiceInput:
		thread.VoiceReplyToVoiceInput = false
	case VoiceOutput:
		thread.VoiceOutput = false
	case VoiceLanguage:
		thread.VoiceLanguage = ""
	case VoiceName:
		thread.VoiceName = ""
	case VoiceModel:
		thread.VoiceModel = ""
	case VoiceDeviceTarget:
		thread.VoiceDeviceTarget = ""
	default:
		return "", UnknownKey(key)
	}
	return Value(*thread, key)
}

func NormalizeKey(key string) string {
	return strings.ReplaceAll(strings.TrimSpace(strings.ToLower(key)), "_", "-")
}

func KnownKey(key string) bool {
	key = NormalizeKey(key)
	for _, candidate := range keys {
		if candidate == key {
			return true
		}
	}
	return false
}

func UnknownKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("missing thread config key")
	}
	return fmt.Errorf("unknown thread config %q", NormalizeKey(key))
}

func parseBool(key string, value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on", "enabled":
		return true, nil
	case "false", "0", "no", "off", "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("thread config %s expects true or false", key)
	}
}

func cleanLanguage(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if idx := strings.IndexAny(value, "-_"); idx >= 0 {
		value = value[:idx]
	}
	return value
}
