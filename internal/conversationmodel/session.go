package conversationmodel

import "time"

type ChatSession struct {
	ID uint `gorm:"primaryKey"`

	ChatID   int64 `gorm:"index:idx_chat_session_active"`
	ThreadID int   `gorm:"index:idx_chat_session_active"`
	Active   bool  `gorm:"index:idx_chat_session_active"`

	ProviderType     string
	ProviderThreadID string

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
