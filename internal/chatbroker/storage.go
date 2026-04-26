package chatbroker

import (
	"context"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"gorm.io/gorm"
)

type Thread struct {
	ID modeluuid.UUID `gorm:"primaryKey"`

	ChatID             modeluuid.UUID `gorm:"uniqueIndex:idx_thread_provider"`
	ProviderThreadID   string         `gorm:"uniqueIndex:idx_thread_provider"`
	Active             bool
	AgentProviderType  string
	AgentThreadID      string
	RuntimeName        string `gorm:"column:container_name"`
	WorkspaceHost      string
	HomeHost           string
	ContainerWorkspace string
	ContainerHome      string
	Initialized        bool
	LastError          string

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (t *Thread) ContainerName(cfg *appstate.Config) string {
	if t == nil {
		return ""
	}
	if cfg != nil && !t.ID.IsNull() {
		if name := strings.TrimSpace(cfg.Thread(t.ChatID, t.ID).ContainerName()); name != "" {
			return name
		}
	}
	return strings.TrimSpace(t.RuntimeName)
}

type SessionStorage struct {
	DB *gorm.DB
}

func NewSessionStorage(db *gorm.DB) *SessionStorage {
	return &SessionStorage{DB: db}
}

func (s *SessionStorage) AutoMigrate(ctx context.Context) error {
	return s.DB.WithContext(ctx).AutoMigrate(&Thread{})
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

func (s *SessionStorage) FindThreadByID(ctx context.Context, threadID modeluuid.UUID) (*Thread, error) {
	var thread Thread
	err := s.DB.WithContext(ctx).
		Where("id = ?", threadID).
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
	FindThread(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*Thread, error)
	FindThreadByID(ctx context.Context, threadID modeluuid.UUID) (*Thread, error)
	EnsureThread(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*Thread, error)
	SaveThread(ctx context.Context, thread *Thread) error
}
