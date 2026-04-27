package gormstorage

import (
	"context"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/dbmodel"
	"github.com/bartdeboer/ctgbot/internal/dbstorage"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"gorm.io/gorm"
)

type ThreadStorage struct {
	db *gorm.DB
}

var _ dbstorage.ThreadStorage = (*ThreadStorage)(nil)

func (s *ThreadStorage) GetByID(ctx context.Context, threadID modeluuid.UUID) (*dbmodel.Thread, error) {
	var thread dbmodel.Thread
	err := s.db.WithContext(ctx).Where("id = ?", threadID).First(&thread).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &thread, nil
}

func (s *ThreadStorage) GetByProviderThreadID(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*dbmodel.Thread, error) {
	var thread dbmodel.Thread
	err := s.db.WithContext(ctx).
		Where("chat_id = ? AND provider_thread_id = ?", chatID, strings.TrimSpace(providerThreadID)).
		First(&thread).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &thread, nil
}

func (s *ThreadStorage) EnsureProviderThread(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*dbmodel.Thread, error) {
	thread, err := s.GetByProviderThreadID(ctx, chatID, providerThreadID)
	if err != nil {
		return nil, err
	}
	if thread != nil {
		return thread, nil
	}
	thread = &dbmodel.Thread{
		ID:               modeluuid.New(),
		ChatID:           chatID,
		ProviderThreadID: strings.TrimSpace(providerThreadID),
	}
	if err := s.db.WithContext(ctx).Create(thread).Error; err != nil {
		return nil, err
	}
	return thread, nil
}

func (s *ThreadStorage) Save(ctx context.Context, thread *dbmodel.Thread) error {
	return s.db.WithContext(ctx).Save(thread).Error
}

func (s *ThreadStorage) WorkspaceHost(ctx context.Context, threadID modeluuid.UUID) (string, error) {
	thread, err := s.GetByID(ctx, threadID)
	if thread == nil || err != nil {
		return "", err
	}
	return strings.TrimSpace(thread.WorkspaceHost), nil
}

func (s *ThreadStorage) SetWorkspaceHost(ctx context.Context, threadID modeluuid.UUID, value string) error {
	return s.update(ctx, threadID, "workspace_host", strings.TrimSpace(value))
}

func (s *ThreadStorage) AgentThreadID(ctx context.Context, threadID modeluuid.UUID) (string, error) {
	thread, err := s.GetByID(ctx, threadID)
	if thread == nil || err != nil {
		return "", err
	}
	return strings.TrimSpace(thread.AgentThreadID), nil
}

func (s *ThreadStorage) SetAgentThreadID(ctx context.Context, threadID modeluuid.UUID, value string) error {
	return s.update(ctx, threadID, "agent_thread_id", strings.TrimSpace(value))
}

func (s *ThreadStorage) KeepRunning(ctx context.Context, threadID modeluuid.UUID) (bool, error) {
	thread, err := s.GetByID(ctx, threadID)
	if thread == nil || err != nil {
		return false, err
	}
	return thread.KeepRunning, nil
}

func (s *ThreadStorage) SetKeepRunning(ctx context.Context, threadID modeluuid.UUID, value bool) error {
	return s.update(ctx, threadID, "keep_running", value)
}

func (s *ThreadStorage) update(ctx context.Context, threadID modeluuid.UUID, column string, value any) error {
	return s.db.WithContext(ctx).
		Model(&dbmodel.Thread{}).
		Where("id = ?", threadID).
		Update(column, value).
		Error
}
