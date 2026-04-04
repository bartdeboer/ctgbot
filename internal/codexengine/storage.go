package codexengine

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type ChatSession struct {
	ID uint `gorm:"primaryKey"`

	ChatID   int64 `gorm:"index:idx_codex_chat_session_active"`
	ThreadID int   `gorm:"index:idx_codex_chat_session_active"`
	Active   bool  `gorm:"index:idx_codex_chat_session_active"`

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

type SessionStorage struct {
	DB *gorm.DB
}

func NewSessionStorage(db *gorm.DB) *SessionStorage {
	return &SessionStorage{DB: db}
}

func (s *SessionStorage) AutoMigrate(ctx context.Context) error {
	return s.DB.WithContext(ctx).AutoMigrate(&ChatSession{})
}

func (s *SessionStorage) GetActive(ctx context.Context, chatID int64, threadID int) (*ChatSession, error) {
	var sess ChatSession
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

func (s *SessionStorage) Create(ctx context.Context, sess *ChatSession) error {
	return s.DB.WithContext(ctx).Create(sess).Error
}

func (s *SessionStorage) MarkStopped(ctx context.Context, id uint, lastErr string) error {
	return s.DB.WithContext(ctx).
		Model(&ChatSession{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"active":     false,
			"last_error": lastErr,
		}).Error
}

func (s *SessionStorage) MarkInitialized(ctx context.Context, id uint) error {
	return s.DB.WithContext(ctx).
		Model(&ChatSession{}).
		Where("id = ?", id).
		Update("initialized", true).Error
}

func (s *SessionStorage) MarkError(ctx context.Context, id uint, lastErr string) error {
	return s.DB.WithContext(ctx).
		Model(&ChatSession{}).
		Where("id = ?", id).
		Update("last_error", lastErr).Error
}
