package outlook

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const StoreFilename = "outlook.db"

type store struct{ db *gorm.DB }

type storedMessage struct {
	ID                string `gorm:"primaryKey"`
	OutlookMessageID  string `gorm:"uniqueIndex"`
	ConversationID    string `gorm:"index"`
	InternetMessageID string
	MailboxEmail      string
	FromEmail         string `gorm:"index"`
	FromLabel         string
	Subject           string    `gorm:"index"`
	ReceivedAt        time.Time `gorm:"index"`
	BodyPreview       string
	BodyText          string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (storedMessage) TableName() string { return "outlook_messages" }

func openStore(profilePath string) (*store, error) {
	profilePath = strings.TrimSpace(profilePath)
	if profilePath == "" {
		return nil, fmt.Errorf("missing outlook profile")
	}
	if err := os.MkdirAll(profilePath, 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(profilePath, StoreFilename)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open outlook db: %w", err)
	}
	if err := db.AutoMigrate(&storedMessage{}); err != nil {
		return nil, fmt.Errorf("migrate outlook db: %w", err)
	}
	if err := db.Exec(`DROP VIEW IF EXISTS messages`).Error; err != nil {
		return nil, err
	}
	if err := db.Exec(`CREATE VIEW messages AS SELECT id, outlook_message_id, conversation_id, internet_message_id, mailbox_email, from_email, from_label, subject, received_at, body_preview, created_at, updated_at FROM outlook_messages`).Error; err != nil {
		return nil, err
	}
	return &store{db: db}, nil
}
func newStoreID() string { return modeluuid.New().String() }
func (s *store) saveMessage(ctx context.Context, m *storedMessage) error {
	if m == nil {
		return fmt.Errorf("missing outlook message")
	}
	if strings.TrimSpace(m.ID) == "" {
		m.ID = newStoreID()
	}
	return s.db.WithContext(ctx).Save(m).Error
}
func (s *store) messageByOutlookID(ctx context.Context, id string) (*storedMessage, error) {
	var m storedMessage
	err := s.db.WithContext(ctx).Where("outlook_message_id = ?", strings.TrimSpace(id)).First(&m).Error
	if err == nil {
		return &m, nil
	}
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return nil, err
}
func (s *store) messageByRef(ctx context.Context, ref string) (*storedMessage, error) {
	var m storedMessage
	err := s.db.WithContext(ctx).Where("id = ? OR outlook_message_id = ?", strings.TrimSpace(ref), strings.TrimSpace(ref)).First(&m).Error
	if err == nil {
		return &m, nil
	}
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return nil, err
}
