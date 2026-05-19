package messaging

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	ThreadConfigVoiceReplyToVoiceInput = "voice.reply_to_voice_input"
	ThreadConfigVoiceOutput            = "voice.output"
	ThreadConfigVoiceLanguage          = "voice.language"
	ThreadConfigVoiceName              = "voice.name"
	ThreadConfigVoiceModel             = "voice.model"
)

type ThreadConfig struct {
	VoiceReplyToVoiceInput bool   `json:"voice_reply_to_voice_input,omitempty"`
	VoiceOutput            bool   `json:"voice_output,omitempty"`
	VoiceLanguage          string `json:"voice_language,omitempty"`
	VoiceName              string `json:"voice_name,omitempty"`
	VoiceModel             string `json:"voice_model,omitempty"`
}

type ThreadConfigValue struct {
	Key   string
	Value string
}

func ParseThreadConfig(raw string) (ThreadConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ThreadConfig{}, nil
	}
	var config ThreadConfig
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return ThreadConfig{}, fmt.Errorf("parse thread config: %w", err)
	}
	config.VoiceLanguage = cleanLanguageCode(config.VoiceLanguage)
	config.VoiceName = strings.TrimSpace(config.VoiceName)
	config.VoiceModel = strings.TrimSpace(config.VoiceModel)
	return config, nil
}

func (c ThreadConfig) JSON() (string, error) {
	c.VoiceLanguage = cleanLanguageCode(c.VoiceLanguage)
	c.VoiceName = strings.TrimSpace(c.VoiceName)
	c.VoiceModel = strings.TrimSpace(c.VoiceModel)
	if c == (ThreadConfig{}) {
		return "", nil
	}
	blob, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(blob), nil
}

func (c ThreadConfig) Values() []ThreadConfigValue {
	values := []ThreadConfigValue{
		{Key: ThreadConfigVoiceReplyToVoiceInput, Value: strconv.FormatBool(c.VoiceReplyToVoiceInput)},
		{Key: ThreadConfigVoiceOutput, Value: strconv.FormatBool(c.VoiceOutput)},
		{Key: ThreadConfigVoiceLanguage, Value: c.VoiceLanguage},
		{Key: ThreadConfigVoiceName, Value: c.VoiceName},
		{Key: ThreadConfigVoiceModel, Value: c.VoiceModel},
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Key < values[j].Key })
	return values
}

func (c ThreadConfig) Value(key string) (string, bool) {
	switch normalizeThreadConfigKey(key) {
	case ThreadConfigVoiceReplyToVoiceInput:
		return strconv.FormatBool(c.VoiceReplyToVoiceInput), true
	case ThreadConfigVoiceOutput:
		return strconv.FormatBool(c.VoiceOutput), true
	case ThreadConfigVoiceLanguage:
		return c.VoiceLanguage, true
	case ThreadConfigVoiceName:
		return c.VoiceName, true
	case ThreadConfigVoiceModel:
		return c.VoiceModel, true
	default:
		return "", false
	}
}

func (c *ThreadConfig) Set(key string, value string) error {
	if c == nil {
		return fmt.Errorf("missing thread config")
	}
	key = normalizeThreadConfigKey(key)
	value = strings.TrimSpace(value)
	switch key {
	case ThreadConfigVoiceReplyToVoiceInput:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("thread config %s expects true or false", key)
		}
		c.VoiceReplyToVoiceInput = parsed
	case ThreadConfigVoiceOutput:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("thread config %s expects true or false", key)
		}
		c.VoiceOutput = parsed
	case ThreadConfigVoiceLanguage:
		c.VoiceLanguage = cleanLanguageCode(value)
	case ThreadConfigVoiceName:
		c.VoiceName = value
	case ThreadConfigVoiceModel:
		c.VoiceModel = value
	default:
		return UnknownThreadConfigKey(key)
	}
	return nil
}

func UnknownThreadConfigKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("missing thread config key")
	}
	return fmt.Errorf("unknown thread config %q", key)
}

func normalizeThreadConfigKey(key string) string {
	return strings.TrimSpace(strings.ToLower(key))
}

func cleanLanguageCode(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if idx := strings.IndexAny(value, "-_"); idx >= 0 {
		value = value[:idx]
	}
	return value
}
