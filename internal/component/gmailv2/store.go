package gmailv2

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

const StoreFilename = "gmailv2.db"

type store struct{ db *gorm.DB }

type storedMessage struct {
	ID             string `gorm:"primaryKey"`
	GmailMessageID string `gorm:"uniqueIndex"`
	GmailThreadID  string `gorm:"index"`
	RFCMessageID   string
	MailboxEmail   string
	FromEmail      string `gorm:"index"`
	FromLabel      string
	ToJSON         string
	CcJSON         string
	BccJSON        string
	Subject        string    `gorm:"index"`
	Date           time.Time `gorm:"index"`
	Snippet        string
	HeadersJSON    string
	HasText        bool
	HasRaw         bool
	HasHTML        bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (storedMessage) TableName() string { return "gmail_messages" }

type storedAttachment struct {
	ID                string `gorm:"primaryKey"`
	MessageID         string `gorm:"index"`
	GmailAttachmentID string
	Kind              string
	Filename          string
	ContentType       string
	Disposition       string
	ContentID         string
	Size              int64
	CreatedAt         time.Time
}

func (storedAttachment) TableName() string { return "gmail_attachments" }

type senderPolicy struct {
	ID          string `gorm:"primaryKey"`
	SenderEmail string `gorm:"uniqueIndex"`
	Trusted     bool
	ShowFull    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (senderPolicy) TableName() string { return "gmail_sender_policies" }

func openStore(homePath string, defaultShowFull bool) (*store, error) {
	homePath = strings.TrimSpace(homePath)
	if homePath == "" {
		return nil, fmt.Errorf("missing gmailv2 profile home")
	}
	if err := os.MkdirAll(homePath, 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(homePath, StoreFilename)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open gmailv2 db: %w", err)
	}
	if err := db.AutoMigrate(&storedMessage{}, &storedAttachment{}, &senderPolicy{}); err != nil {
		return nil, fmt.Errorf("migrate gmailv2 db: %w", err)
	}
	if err := migrateViews(db, defaultShowFull); err != nil {
		return nil, err
	}
	return &store{db: db}, nil
}

func migrateViews(db *gorm.DB, defaultShowFull bool) error {
	defaultShowFullSQL := "false"
	if defaultShowFull {
		defaultShowFullSQL = "true"
	}
	statements := []string{
		`DROP VIEW IF EXISTS messages`,
		`CREATE VIEW messages AS
SELECT
  m.id,
  m.gmail_message_id,
  m.gmail_thread_id,
  m.rfc_message_id,
  m.mailbox_email,
  m.from_email,
  m.from_label,
  m.subject,
  m.date,
  m.snippet,
  m.has_text,
  m.has_raw,
  m.has_html,
  COALESCE(p.trusted, false) AS trusted,
  COALESCE(p.show_full, ` + defaultShowFullSQL + `) AS show_full
FROM gmail_messages m
LEFT JOIN gmail_sender_policies p ON p.sender_email = m.from_email`,
		`DROP VIEW IF EXISTS attachments`,
		`CREATE VIEW attachments AS
SELECT id, message_id, filename, content_type, disposition, content_id, size
FROM gmail_attachments`,
		`DROP VIEW IF EXISTS sender_policies`,
		`CREATE VIEW sender_policies AS
SELECT sender_email, trusted, show_full, updated_at
FROM gmail_sender_policies`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("migrate gmailv2 view: %w", err)
		}
	}
	return nil
}

func newStoreID() string { return modeluuid.New().String() }

func (s *store) saveMessage(ctx context.Context, message *storedMessage) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("missing gmailv2 store")
	}
	if message == nil {
		return fmt.Errorf("missing gmailv2 message")
	}
	if strings.TrimSpace(message.ID) == "" {
		message.ID = newStoreID()
	}
	return s.db.WithContext(ctx).Save(message).Error
}

func (s *store) messageByGmailID(ctx context.Context, gmailID string) (*storedMessage, error) {
	var message storedMessage
	err := s.db.WithContext(ctx).Where("gmail_message_id = ?", strings.TrimSpace(gmailID)).First(&message).Error
	if err == nil {
		return &message, nil
	}
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return nil, err
}

func (s *store) messageByID(ctx context.Context, id string) (*storedMessage, error) {
	var message storedMessage
	err := s.db.WithContext(ctx).First(&message, "id = ?", strings.TrimSpace(id)).Error
	if err == nil {
		return &message, nil
	}
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return nil, err
}

func (s *store) saveAttachment(ctx context.Context, attachment *storedAttachment) error {
	if strings.TrimSpace(attachment.ID) == "" {
		attachment.ID = newStoreID()
	}
	return s.db.WithContext(ctx).Save(attachment).Error
}

func (s *store) attachmentByID(ctx context.Context, id string) (*storedAttachment, error) {
	var attachment storedAttachment
	err := s.db.WithContext(ctx).First(&attachment, "id = ?", strings.TrimSpace(id)).Error
	if err == nil {
		return &attachment, nil
	}
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return nil, err
}

func (s *store) attachmentsByMessage(ctx context.Context, messageID string) ([]storedAttachment, error) {
	var out []storedAttachment
	return out, s.db.WithContext(ctx).Where("message_id = ?", strings.TrimSpace(messageID)).Order("created_at asc, id asc").Find(&out).Error
}

func (s *store) senderPolicy(ctx context.Context, email string) (*senderPolicy, error) {
	var policy senderPolicy
	err := s.db.WithContext(ctx).First(&policy, "sender_email = ?", normalizeEmail(email)).Error
	if err == nil {
		return &policy, nil
	}
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return nil, err
}

func (s *store) saveSenderPolicy(ctx context.Context, email string, update func(*senderPolicy)) error {
	email = normalizeEmail(email)
	if email == "" {
		return fmt.Errorf("missing sender email")
	}
	policy, err := s.senderPolicy(ctx, email)
	if err != nil {
		return err
	}
	if policy == nil {
		policy = &senderPolicy{ID: newStoreID(), SenderEmail: email}
	}
	update(policy)
	return s.db.WithContext(ctx).Save(policy).Error
}

func (s *store) listSenderPolicies(ctx context.Context) ([]senderPolicy, error) {
	var out []senderPolicy
	return out, s.db.WithContext(ctx).Order("sender_email asc").Find(&out).Error
}

func (s *store) deleteSenderPolicy(ctx context.Context, email string) (bool, error) {
	result := s.db.WithContext(ctx).Where("sender_email = ?", normalizeEmail(email)).Delete(&senderPolicy{})
	return result.RowsAffected > 0, result.Error
}
