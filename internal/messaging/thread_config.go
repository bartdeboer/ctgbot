package messaging

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

const (
	ThreadConfigVoiceReplyToVoiceInput = "voice.reply-to-voice-input"
	ThreadConfigVoiceOutput            = "voice.output"
	ThreadConfigVoiceLanguage          = "voice.language"
	ThreadConfigVoiceName              = "voice.name"
	ThreadConfigVoiceModel             = "voice.model"
	ThreadConfigVoiceDeviceTarget      = "voice.device-target"
)

var threadConfigKeys = []string{
	ThreadConfigVoiceReplyToVoiceInput,
	ThreadConfigVoiceOutput,
	ThreadConfigVoiceLanguage,
	ThreadConfigVoiceName,
	ThreadConfigVoiceModel,
	ThreadConfigVoiceDeviceTarget,
}

func (s *Service) ThreadConfigList(ctx context.Context, actor coremodel.Actor, threadID modeluuid.UUID) ([]string, error) {
	thread, err := s.threadConfigThread(ctx, actor, threadID)
	if err != nil {
		return nil, err
	}
	lines := make([]string, 0, len(threadConfigKeys))
	for _, key := range threadConfigKeys {
		lines = append(lines, "thread config "+key+"="+threadConfigValue(*thread, key))
	}
	return lines, nil
}

func (s *Service) ThreadConfigGet(ctx context.Context, actor coremodel.Actor, threadID modeluuid.UUID, key string) (string, string, error) {
	thread, err := s.threadConfigThread(ctx, actor, threadID)
	if err != nil {
		return "", "", err
	}
	key = normalizeThreadConfigKey(key)
	if !knownThreadConfig(key) {
		return "", "", unknownThreadConfig(key)
	}
	return key, threadConfigValue(*thread, key), nil
}

func (s *Service) ThreadConfigSet(ctx context.Context, actor coremodel.Actor, threadID modeluuid.UUID, key string, value string) (string, string, error) {
	if err := s.ensureStorage(); err != nil {
		return "", "", err
	}
	if err := requireActor(actor); err != nil {
		return "", "", err
	}
	if !actor.HasRole(simplerbac.RoleRoot) && !actor.HasRole(simplerbac.RoleAgent) {
		return "", "", fmt.Errorf("set thread config denied: missing role")
	}
	key = normalizeThreadConfigKey(key)
	if !knownThreadConfig(key) {
		return "", "", unknownThreadConfig(key)
	}
	value = strings.TrimSpace(value)
	var updatedValue string
	if err := s.Storage.Transaction(ctx, func(tx repository.Storage) error {
		thread, err := tx.Threads().GetByID(ctx, threadID)
		if err != nil {
			return err
		}
		if thread == nil {
			return fmt.Errorf("thread not found: %s", threadID)
		}
		if err := setThreadConfigValue(thread, key, value); err != nil {
			return err
		}
		updatedValue = threadConfigValue(*thread, key)
		return tx.Threads().Save(ctx, thread)
	}); err != nil {
		return "", "", err
	}
	return key, updatedValue, nil
}

func (s *Service) threadConfigThread(ctx context.Context, actor coremodel.Actor, threadID modeluuid.UUID) (*coremodel.Thread, error) {
	if err := s.ensureStorage(); err != nil {
		return nil, err
	}
	if err := requireActor(actor); err != nil {
		return nil, err
	}
	thread, _, err := s.loadThreadAndChat(ctx, threadID)
	return thread, err
}

func setThreadConfigValue(thread *coremodel.Thread, key string, value string) error {
	switch key {
	case ThreadConfigVoiceReplyToVoiceInput:
		parsed, err := parseThreadConfigBool(key, value)
		if err != nil {
			return err
		}
		thread.VoiceReplyToVoiceInput = parsed
	case ThreadConfigVoiceOutput:
		parsed, err := parseThreadConfigBool(key, value)
		if err != nil {
			return err
		}
		thread.VoiceOutput = parsed
	case ThreadConfigVoiceLanguage:
		thread.VoiceLanguage = cleanThreadConfigLanguage(value)
	case ThreadConfigVoiceName:
		thread.VoiceName = strings.TrimSpace(value)
	case ThreadConfigVoiceModel:
		thread.VoiceModel = strings.TrimSpace(value)
	case ThreadConfigVoiceDeviceTarget:
		thread.VoiceDeviceTarget = strings.TrimSpace(value)
	default:
		return unknownThreadConfig(key)
	}
	return nil
}

func threadConfigValue(thread coremodel.Thread, key string) string {
	switch key {
	case ThreadConfigVoiceReplyToVoiceInput:
		return strconv.FormatBool(thread.VoiceReplyToVoiceInput)
	case ThreadConfigVoiceOutput:
		return strconv.FormatBool(thread.VoiceOutput)
	case ThreadConfigVoiceLanguage:
		return strings.TrimSpace(thread.VoiceLanguage)
	case ThreadConfigVoiceName:
		return strings.TrimSpace(thread.VoiceName)
	case ThreadConfigVoiceModel:
		return strings.TrimSpace(thread.VoiceModel)
	case ThreadConfigVoiceDeviceTarget:
		return strings.TrimSpace(thread.VoiceDeviceTarget)
	default:
		return ""
	}
}

func normalizeThreadConfigKey(key string) string {
	return strings.ReplaceAll(strings.TrimSpace(strings.ToLower(key)), "_", "-")
}

func knownThreadConfig(key string) bool {
	for _, candidate := range threadConfigKeys {
		if candidate == key {
			return true
		}
	}
	return false
}

func unknownThreadConfig(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("missing thread config key")
	}
	return fmt.Errorf("unknown thread config %q", key)
}

func parseThreadConfigBool(key string, value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on", "enabled":
		return true, nil
	case "false", "0", "no", "off", "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("thread config %s expects true or false", key)
	}
}

func cleanThreadConfigLanguage(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if idx := strings.IndexAny(value, "-_"); idx >= 0 {
		value = value[:idx]
	}
	return value
}
