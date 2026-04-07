package chatbroker

import (
	"context"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"gorm.io/gorm"
)

type Chat struct {
	ID modeluuid.UUID `gorm:"primaryKey"`

	ProviderType   string `gorm:"uniqueIndex:idx_chat_provider"`
	ProviderChatID string `gorm:"uniqueIndex:idx_chat_provider"`
	Label          string
	Enabled        bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

type Thread struct {
	ID modeluuid.UUID `gorm:"primaryKey"`

	ChatID             modeluuid.UUID `gorm:"uniqueIndex:idx_thread_provider"`
	ProviderThreadID   string         `gorm:"uniqueIndex:idx_thread_provider"`
	Active             bool
	ProviderType       string
	AgentThreadID      string
	ContainerName      string
	WorkspaceHost      string
	HomeHost           string
	ContainerWorkspace string
	ContainerHome      string
	Initialized        bool
	LastError          string

	CreatedAt time.Time
	UpdatedAt time.Time
}

type SessionStorage struct {
	DB *gorm.DB
}

func NewSessionStorage(db *gorm.DB) *SessionStorage {
	return &SessionStorage{DB: db}
}

func (s *SessionStorage) AutoMigrate(ctx context.Context) error {
	return s.DB.WithContext(ctx).AutoMigrate(&Chat{}, &Thread{})
}

func (s *SessionStorage) FindChat(ctx context.Context, providerType string, providerChatID string) (*Chat, error) {
	var chat Chat
	err := s.DB.WithContext(ctx).
		Where("provider_type = ? AND provider_chat_id = ?", strings.TrimSpace(providerType), strings.TrimSpace(providerChatID)).
		First(&chat).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &chat, nil
}

func (s *SessionStorage) GetChatByID(ctx context.Context, id modeluuid.UUID) (*Chat, error) {
	var chat Chat
	err := s.DB.WithContext(ctx).Where("id = ?", id).First(&chat).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &chat, nil
}

func (s *SessionStorage) EnsureChat(ctx context.Context, providerType string, providerChatID string, label string) (*Chat, error) {
	chat, err := s.FindChat(ctx, providerType, providerChatID)
	if err != nil {
		return nil, err
	}
	if chat != nil {
		label = strings.TrimSpace(label)
		if label != "" && chat.Label != label {
			chat.Label = label
			if err := s.DB.WithContext(ctx).Model(&Chat{}).Where("id = ?", chat.ID).Update("label", chat.Label).Error; err != nil {
				return nil, err
			}
		}
		return chat, nil
	}
	chat = &Chat{
		ID:             modeluuid.New(),
		ProviderType:   strings.TrimSpace(providerType),
		ProviderChatID: strings.TrimSpace(providerChatID),
		Label:          strings.TrimSpace(label),
		Enabled:        true,
	}
	if err := s.DB.WithContext(ctx).Create(chat).Error; err != nil {
		return nil, err
	}
	return chat, nil
}

func (s *SessionStorage) FindThread(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*Thread, error) {
	var thread Thread
	err := s.DB.WithContext(ctx).
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

func (s *SessionStorage) EnsureThread(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*Thread, error) {
	thread, err := s.FindThread(ctx, chatID, providerThreadID)
	if err != nil {
		return nil, err
	}
	if thread != nil {
		return thread, nil
	}
	thread = &Thread{
		ID:               modeluuid.New(),
		ChatID:           chatID,
		ProviderThreadID: strings.TrimSpace(providerThreadID),
	}
	if err := s.DB.WithContext(ctx).Create(thread).Error; err != nil {
		return nil, err
	}
	return thread, nil
}

func (s *SessionStorage) SaveThread(ctx context.Context, thread *Thread) error {
	return s.DB.WithContext(ctx).Save(thread).Error
}

type SessionStore interface {
	AutoMigrate(ctx context.Context) error
	FindChat(ctx context.Context, providerType string, providerChatID string) (*Chat, error)
	GetChatByID(ctx context.Context, id modeluuid.UUID) (*Chat, error)
	EnsureChat(ctx context.Context, providerType string, providerChatID string, label string) (*Chat, error)
	FindThread(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*Thread, error)
	EnsureThread(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*Thread, error)
	SaveThread(ctx context.Context, thread *Thread) error
}
