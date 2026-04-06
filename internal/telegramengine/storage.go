package telegramengine

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/chatmodel"
	"github.com/bartdeboer/ctgbot/internal/conversationmodel"
	"gorm.io/gorm"
)

type UpdateStorage struct {
	DB *gorm.DB
}

func NewUpdateStorage(db *gorm.DB) *UpdateStorage {
	return &UpdateStorage{DB: db}
}

func (s *UpdateStorage) AutoMigrate(ctx context.Context) error {
	return s.DB.WithContext(ctx).AutoMigrate(&chatmodel.TelegramUpdate{})
}

func (s *UpdateStorage) Create(ctx context.Context, event *chatmodel.TelegramUpdate) error {
	return s.DB.WithContext(ctx).Create(event).Error
}

func (s *UpdateStorage) Save(ctx context.Context, event *chatmodel.TelegramUpdate) error {
	return s.DB.WithContext(ctx).Save(event).Error
}

type SessionStorage struct {
	DB *gorm.DB
}

func NewSessionStorage(db *gorm.DB) *SessionStorage {
	return &SessionStorage{DB: db}
}

func (s *SessionStorage) AutoMigrate(ctx context.Context) error {
	return s.DB.WithContext(ctx).AutoMigrate(&conversationmodel.ChatSession{})
}

func (s *SessionStorage) GetActive(ctx context.Context, chatID int64, threadID int) (*conversationmodel.ChatSession, error) {
	var sess conversationmodel.ChatSession
	err := s.DB.WithContext(ctx).
		Where("chat_id = ? AND thread_id = ? AND active = ?", chatID, threadID, true).
		Order("id desc").
		First(&sess).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &sess, nil
}

func (s *SessionStorage) Create(ctx context.Context, sess *conversationmodel.ChatSession) error {
	return s.DB.WithContext(ctx).Create(sess).Error
}

func (s *SessionStorage) MarkStopped(ctx context.Context, id uint, lastErr string) error {
	return s.DB.WithContext(ctx).
		Model(&conversationmodel.ChatSession{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"active":     false,
			"last_error": lastErr,
		}).Error
}

func (s *SessionStorage) MarkInitialized(ctx context.Context, id uint) error {
	return s.DB.WithContext(ctx).
		Model(&conversationmodel.ChatSession{}).
		Where("id = ?", id).
		Update("initialized", true).Error
}

func (s *SessionStorage) MarkError(ctx context.Context, id uint, lastErr string) error {
	return s.DB.WithContext(ctx).
		Model(&conversationmodel.ChatSession{}).
		Where("id = ?", id).
		Update("last_error", lastErr).Error
}

func (s *SessionStorage) MarkProviderThreadID(ctx context.Context, id uint, threadID string) error {
	return s.DB.WithContext(ctx).
		Model(&conversationmodel.ChatSession{}).
		Where("id = ?", id).
		Update("provider_thread_id", threadID).Error
}
