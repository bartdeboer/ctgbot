package theater

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

const StoreFilename = "theater.db"

type store struct{ db *gorm.DB }

type theaterRecord struct {
	ID            string `gorm:"primaryKey"`
	Name          string `gorm:"uniqueIndex"`
	Label         string
	ThreadID      string `gorm:"index"`
	WorkspacePath string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (theaterRecord) TableName() string { return "theaters" }

type subscriptionRecord struct {
	ID         string `gorm:"primaryKey"`
	TheaterID  string `gorm:"uniqueIndex:idx_theater_subscription;index"`
	ThreadID   string `gorm:"uniqueIndex:idx_theater_subscription;index"`
	LastReadAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (subscriptionRecord) TableName() string { return "theater_subscriptions" }

func openStore(homePath string) (*store, error) {
	homePath = strings.TrimSpace(homePath)
	if homePath == "" {
		return nil, fmt.Errorf("missing theater component home")
	}
	if err := os.MkdirAll(homePath, 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(homePath, StoreFilename)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open theater db: %w", err)
	}
	if err := db.AutoMigrate(&theaterRecord{}, &subscriptionRecord{}); err != nil {
		return nil, fmt.Errorf("migrate theater db: %w", err)
	}
	return &store{db: db}, nil
}

func newID() string { return modeluuid.New().String() }

func normalizeName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (s *store) createTheater(ctx context.Context, name string, workspacePath string, threadID modeluuid.UUID) (theaterRecord, bool, error) {
	if s == nil || s.db == nil {
		return theaterRecord{}, false, fmt.Errorf("missing theater store")
	}
	name = normalizeName(name)
	workspacePath = strings.TrimSpace(workspacePath)
	if name == "" {
		return theaterRecord{}, false, fmt.Errorf("missing theater name")
	}
	var existing theaterRecord
	if err := s.db.WithContext(ctx).Where("name = ?", name).First(&existing).Error; err == nil {
		changed := false
		if workspacePath != "" && strings.TrimSpace(existing.WorkspacePath) != workspacePath {
			existing.WorkspacePath = workspacePath
			changed = true
		}
		// Creating an existing theater may fill in a missing thread binding, but
		// never overwrites an explicit binding. Use theater <name> bind for that.
		if !threadID.IsNull() && strings.TrimSpace(existing.ThreadID) == "" {
			existing.ThreadID = threadID.String()
			changed = true
		}
		if changed {
			if err := s.db.WithContext(ctx).Save(&existing).Error; err != nil {
				return theaterRecord{}, false, err
			}
		}
		return existing, false, nil
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return theaterRecord{}, false, err
	}
	record := theaterRecord{ID: newID(), Name: name, Label: name, WorkspacePath: workspacePath}
	if !threadID.IsNull() {
		record.ThreadID = threadID.String()
	}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return theaterRecord{}, false, err
	}
	return record, true, nil
}

func (s *store) theaterByName(ctx context.Context, name string) (theaterRecord, error) {
	if s == nil || s.db == nil {
		return theaterRecord{}, fmt.Errorf("missing theater store")
	}
	name = normalizeName(name)
	if name == "" {
		return theaterRecord{}, fmt.Errorf("missing theater name")
	}
	var record theaterRecord
	if err := s.db.WithContext(ctx).Where("name = ?", name).First(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return theaterRecord{}, fmt.Errorf("theater not found: %s", name)
		}
		return theaterRecord{}, err
	}
	return record, nil
}

func (s *store) theaterByID(ctx context.Context, id string) (theaterRecord, error) {
	if s == nil || s.db == nil {
		return theaterRecord{}, fmt.Errorf("missing theater store")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return theaterRecord{}, fmt.Errorf("missing theater id")
	}
	var record theaterRecord
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return theaterRecord{}, fmt.Errorf("theater not found: %s", id)
		}
		return theaterRecord{}, err
	}
	return record, nil
}

func (s *store) listTheaters(ctx context.Context) ([]theaterRecord, error) {
	var records []theaterRecord
	if err := s.db.WithContext(ctx).Order("name asc").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (s *store) bindThread(ctx context.Context, theater theaterRecord, threadID modeluuid.UUID) error {
	if threadID.IsNull() {
		return fmt.Errorf("missing thread id")
	}
	theater.ThreadID = threadID.String()
	return s.db.WithContext(ctx).Save(&theater).Error
}

func (s *store) subscribe(ctx context.Context, theater theaterRecord, threadID modeluuid.UUID) (bool, error) {
	if threadID.IsNull() {
		return false, fmt.Errorf("missing thread id")
	}
	var existing subscriptionRecord
	err := s.db.WithContext(ctx).Where("theater_id = ? and thread_id = ?", theater.ID, threadID.String()).First(&existing).Error
	if err == nil {
		return false, nil
	}
	if err != gorm.ErrRecordNotFound {
		return false, err
	}
	record := subscriptionRecord{ID: newID(), TheaterID: theater.ID, ThreadID: threadID.String()}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return false, err
	}
	return true, nil
}

func (s *store) unsubscribe(ctx context.Context, theater theaterRecord, threadID modeluuid.UUID) (bool, error) {
	result := s.db.WithContext(ctx).Where("theater_id = ? and thread_id = ?", theater.ID, threadID.String()).Delete(&subscriptionRecord{})
	return result.RowsAffected > 0, result.Error
}

func (s *store) subscriptions(ctx context.Context, threadID modeluuid.UUID) ([]subscriptionRecord, error) {
	var records []subscriptionRecord
	if threadID.IsNull() {
		return nil, fmt.Errorf("missing thread id")
	}
	if err := s.db.WithContext(ctx).Where("thread_id = ?", threadID.String()).Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (s *store) subscription(ctx context.Context, theater theaterRecord, threadID modeluuid.UUID) (subscriptionRecord, bool, error) {
	var record subscriptionRecord
	if threadID.IsNull() {
		return subscriptionRecord{}, false, nil
	}
	if err := s.db.WithContext(ctx).Where("theater_id = ? and thread_id = ?", theater.ID, threadID.String()).First(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return subscriptionRecord{}, false, nil
		}
		return subscriptionRecord{}, false, err
	}
	return record, true, nil
}

func (s *store) markRead(ctx context.Context, theater theaterRecord, threadID modeluuid.UUID, at time.Time) error {
	if threadID.IsNull() || at.IsZero() {
		return nil
	}
	return s.db.WithContext(ctx).Model(&subscriptionRecord{}).Where("theater_id = ? and thread_id = ?", theater.ID, threadID.String()).Update("last_read_at", at.UTC()).Error
}
