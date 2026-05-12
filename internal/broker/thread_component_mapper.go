package broker

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

type ThreadComponentMapper interface {
	EnsureThread(ctx context.Context, binding coremodel.ChatComponent, componentThreadID string) (*coremodel.Thread, error)
	ComponentThreadID(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) (string, bool, error)
	BindComponentThreadID(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID, componentThreadID string) error
	RelayTarget(ctx context.Context, threadID modeluuid.UUID, binding coremodel.ChatComponent) (*message.ChatTarget, bool, error)
}

type threadComponentMapper struct {
	storage repository.Storage
}

func NewThreadComponentMapper(storage repository.Storage) ThreadComponentMapper {
	return &threadComponentMapper{storage: storage}
}

func (m *threadComponentMapper) EnsureThread(ctx context.Context, binding coremodel.ChatComponent, componentThreadID string) (*coremodel.Thread, error) {
	if m == nil || m.storage == nil {
		return nil, fmt.Errorf("missing thread component mapper storage")
	}
	if binding.ChatID.IsNull() {
		return nil, fmt.Errorf("missing chat id")
	}
	if binding.ComponentID.IsNull() {
		return nil, fmt.Errorf("missing component id")
	}
	componentThreadID = normalizeComponentThreadID(componentThreadID)

	mapping, err := m.storage.ThreadComponentMappings().FindByChatComponentAndThreadID(ctx, binding.ChatID, binding.ComponentID, componentThreadID)
	if err != nil {
		return nil, err
	}
	if mapping != nil {
		thread, err := m.storage.Threads().GetByID(ctx, mapping.ThreadID)
		if err != nil {
			return nil, err
		}
		if thread == nil {
			return nil, fmt.Errorf("thread mapping %s points to missing thread %s", mapping.ID, mapping.ThreadID)
		}
		return thread, nil
	}

	if reusable, err := m.reusableVisibleThread(ctx, binding, componentThreadID); err != nil {
		return nil, err
	} else if reusable != nil {
		if err := m.BindComponentThreadID(ctx, reusable.ID, binding.ComponentID, componentThreadID); err != nil {
			return nil, err
		}
		return reusable, nil
	}

	thread := &coremodel.Thread{
		ChatID: binding.ChatID,
	}
	if componentThreadID != defaultComponentThreadID {
		thread.Label = "thread " + componentThreadID
	}
	if err := m.storage.Threads().Save(ctx, thread); err != nil {
		return nil, err
	}
	if err := m.BindComponentThreadID(ctx, thread.ID, binding.ComponentID, componentThreadID); err != nil {
		return nil, err
	}
	return thread, nil
}

func (m *threadComponentMapper) reusableVisibleThread(ctx context.Context, binding coremodel.ChatComponent, componentThreadID string) (*coremodel.Thread, error) {
	if !isMailboxComponentThreadID(componentThreadID) {
		return nil, nil
	}
	mappings, err := m.storage.ThreadComponentMappings().ListByChatID(ctx, binding.ChatID)
	if err != nil {
		return nil, err
	}
	for _, mapping := range mappings {
		if mapping.ComponentID == binding.ComponentID {
			continue
		}
		if !isVisibleDefaultComponentThreadID(mapping.ComponentThreadID) {
			continue
		}
		thread, err := m.storage.Threads().GetByID(ctx, mapping.ThreadID)
		if err != nil {
			return nil, err
		}
		if thread != nil {
			return thread, nil
		}
	}
	return nil, nil
}

func (m *threadComponentMapper) ComponentThreadID(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) (string, bool, error) {
	if m == nil || m.storage == nil {
		return "", false, fmt.Errorf("missing thread component mapper storage")
	}
	if threadID.IsNull() || componentID.IsNull() {
		return "", false, nil
	}
	mapping, err := m.storage.ThreadComponentMappings().GetByThreadAndComponent(ctx, threadID, componentID)
	if err != nil {
		return "", false, err
	}
	if mapping == nil {
		return "", false, nil
	}
	return strings.TrimSpace(mapping.ComponentThreadID), true, nil
}

func (m *threadComponentMapper) BindComponentThreadID(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID, componentThreadID string) error {
	if m == nil || m.storage == nil {
		return fmt.Errorf("missing thread component mapper storage")
	}
	if threadID.IsNull() {
		return fmt.Errorf("missing thread id")
	}
	if componentID.IsNull() {
		return fmt.Errorf("missing component id")
	}
	componentThreadID = normalizeComponentThreadID(componentThreadID)

	mapping, err := m.storage.ThreadComponentMappings().GetByThreadAndComponent(ctx, threadID, componentID)
	if err != nil {
		return err
	}
	if mapping == nil {
		thread, err := m.storage.Threads().GetByID(ctx, threadID)
		if err != nil {
			return err
		}
		if thread == nil {
			return fmt.Errorf("missing thread %s", threadID)
		}
		mapping = &coremodel.ThreadComponentMapping{
			ThreadID:    threadID,
			ChatID:      thread.ChatID,
			ComponentID: componentID,
		}
	}
	mapping.ComponentThreadID = componentThreadID
	return m.storage.ThreadComponentMappings().Save(ctx, mapping)
}

func (m *threadComponentMapper) RelayTarget(ctx context.Context, threadID modeluuid.UUID, binding coremodel.ChatComponent) (*message.ChatTarget, bool, error) {
	componentThreadID, ok, err := m.ComponentThreadID(ctx, threadID, binding.ComponentID)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		externalChatID := strings.TrimSpace(binding.ExternalChatID)
		if externalChatID == "" {
			return nil, false, nil
		}
		return &message.ChatTarget{
			ProviderChatID: externalChatID,
		}, true, nil
	}
	return &message.ChatTarget{
		ProviderChatID:   strings.TrimSpace(binding.ExternalChatID),
		ProviderThreadID: componentThreadID,
	}, true, nil
}

const defaultComponentThreadID = "default"

func normalizeComponentThreadID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultComponentThreadID
	}
	return value
}

func isMailboxComponentThreadID(value string) bool {
	return strings.Contains(strings.TrimSpace(value), "@")
}

func isVisibleDefaultComponentThreadID(value string) bool {
	switch strings.TrimSpace(value) {
	case "0", defaultComponentThreadID:
		return true
	default:
		return false
	}
}
