package dbmodel

import (
	"time"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type Thread struct {
	ID modeluuid.UUID `gorm:"primaryKey"`

	ChatID             modeluuid.UUID `gorm:"uniqueIndex:idx_thread_provider"`
	ProviderThreadID   string         `gorm:"uniqueIndex:idx_thread_provider"`
	Active             bool
	AgentProviderType  string
	AgentThreadID      string
	RuntimeName        string `gorm:"column:container_name"`
	KeepRunning        bool
	WorkspaceHost      string
	HomeHost           string
	ContainerWorkspace string
	ContainerHome      string
	Initialized        bool
	LastError          string

	CreatedAt time.Time
	UpdatedAt time.Time
}
