package botengine

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type Conversation struct {
	ID uint `gorm:"primaryKey"`

	ChatID   int64 `gorm:"index:idx_chat_thread_status"`
	ThreadID int   `gorm:"index:idx_chat_thread_status"`
	Status   string

	ContainerName string
	WorkspaceHost string
	HomeHost      string

	ContainerWorkspace string
	ContainerHome      string

	Initialized bool
	LastError   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ConversationStorage struct {
	DB *gorm.DB
}

func NewConversationStorage(db *gorm.DB) *ConversationStorage {
	return &ConversationStorage{DB: db}
}

func (s *ConversationStorage) AutoMigrate(ctx context.Context) error {
	return s.DB.WithContext(ctx).AutoMigrate(&Conversation{})
}

func (s *ConversationStorage) GetActive(ctx context.Context, chatID int64, threadID int) (*Conversation, error) {
	var conv Conversation
	err := s.DB.WithContext(ctx).
		Where("chat_id = ? AND thread_id = ? AND status = ?", chatID, threadID, "active").
		Order("id desc").
		First(&conv).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &conv, nil
}

func (s *ConversationStorage) Create(ctx context.Context, conv *Conversation) error {
	return s.DB.WithContext(ctx).Create(conv).Error
}

func (s *ConversationStorage) MarkStopped(ctx context.Context, id uint, lastErr string) error {
	return s.DB.WithContext(ctx).
		Model(&Conversation{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     "stopped",
			"last_error": lastErr,
		}).Error
}

func (s *ConversationStorage) MarkInitialized(ctx context.Context, id uint) error {
	return s.DB.WithContext(ctx).
		Model(&Conversation{}).
		Where("id = ?", id).
		Update("initialized", true).Error
}

func (s *ConversationStorage) MarkError(ctx context.Context, id uint, lastErr string) error {
	return s.DB.WithContext(ctx).
		Model(&Conversation{}).
		Where("id = ?", id).
		Update("last_error", lastErr).Error
}
