package messaging

import (
	"context"
	"fmt"

	threadconfig "github.com/bartdeboer/ctgbot/internal/app/config/thread"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

const (
	ThreadConfigVoiceReplyToVoiceInput = threadconfig.VoiceReplyToVoiceInput
	ThreadConfigVoiceOutput            = threadconfig.VoiceOutput
	ThreadConfigVoiceLanguage          = threadconfig.VoiceLanguage
	ThreadConfigVoiceName              = threadconfig.VoiceName
	ThreadConfigVoiceModel             = threadconfig.VoiceModel
	ThreadConfigVoiceDeviceTarget      = threadconfig.VoiceDeviceTarget
)

func (s *Service) ThreadConfigList(ctx context.Context, actor coremodel.Actor, threadID modeluuid.UUID) (string, error) {
	thread, err := s.threadConfigThread(ctx, actor, threadID)
	if err != nil {
		return "", err
	}
	surface := threadconfig.NewSurface(thread)
	return configsurface.FormatList(ctx, commandengine.Request{}, surface, threadconfig.Schema()), nil
}

func (s *Service) ThreadConfigGet(ctx context.Context, actor coremodel.Actor, threadID modeluuid.UUID, key string) (string, string, error) {
	thread, err := s.threadConfigThread(ctx, actor, threadID)
	if err != nil {
		return "", "", err
	}
	key = threadconfig.NormalizeKey(key)
	value, err := threadconfig.NewSurface(thread).ConfigGet(ctx, commandengine.Request{}, key)
	if err != nil {
		return "", "", err
	}
	return key, value, nil
}

func (s *Service) ThreadConfigSet(ctx context.Context, actor coremodel.Actor, threadID modeluuid.UUID, key string, value string) (string, string, error) {
	return s.updateThreadConfig(ctx, actor, threadID, key, func(surface *threadconfig.Surface, key string) (string, error) {
		if err := surface.ConfigSet(ctx, commandengine.Request{}, key, value); err != nil {
			return "", err
		}
		return surface.ConfigGet(ctx, commandengine.Request{}, key)
	})
}

func (s *Service) ThreadConfigUnset(ctx context.Context, actor coremodel.Actor, threadID modeluuid.UUID, key string) (string, string, error) {
	return s.updateThreadConfig(ctx, actor, threadID, key, func(surface *threadconfig.Surface, key string) (string, error) {
		if err := surface.ConfigUnset(ctx, commandengine.Request{}, key); err != nil {
			return "", err
		}
		return surface.ConfigGet(ctx, commandengine.Request{}, key)
	})
}

func (s *Service) updateThreadConfig(ctx context.Context, actor coremodel.Actor, threadID modeluuid.UUID, key string, update func(*threadconfig.Surface, string) (string, error)) (string, string, error) {
	if err := s.ensureStorage(); err != nil {
		return "", "", err
	}
	if err := requireActor(actor); err != nil {
		return "", "", err
	}
	if !actor.HasRole(simplerbac.RoleRoot) && !actor.HasRole(simplerbac.RoleAgent) {
		return "", "", fmt.Errorf("set thread config denied: missing role")
	}
	key = threadconfig.NormalizeKey(key)
	if !threadconfig.KnownKey(key) {
		return "", "", threadconfig.UnknownKey(key)
	}
	var updatedValue string
	if err := s.Storage.Transaction(ctx, func(tx repository.Storage) error {
		thread, err := tx.Threads().GetByID(ctx, threadID)
		if err != nil {
			return err
		}
		if thread == nil {
			return fmt.Errorf("thread not found: %s", threadID)
		}
		value, err := update(threadconfig.NewSurface(thread), key)
		if err != nil {
			return err
		}
		updatedValue = value
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
