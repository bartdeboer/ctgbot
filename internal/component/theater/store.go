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

type subscriptionRecord struct {
	ID                 string `gorm:"primaryKey"`
	SubscriberThreadID string `gorm:"uniqueIndex:idx_theater_thread_subscription;index"`
	TargetThreadID     string `gorm:"uniqueIndex:idx_theater_thread_subscription;index"`
	Label              string
	LastReadAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (subscriptionRecord) TableName() string { return "theater_thread_subscriptions" }

func openStore(profilePath string) (*store, error) {
	profilePath = strings.TrimSpace(profilePath)
	if profilePath == "" {
		return nil, fmt.Errorf("missing theater component profile")
	}
	if err := os.MkdirAll(profilePath, 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(profilePath, StoreFilename)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open theater db: %w", err)
	}
	if err := db.AutoMigrate(&subscriptionRecord{}); err != nil {
		return nil, fmt.Errorf("migrate theater db: %w", err)
	}
	return &store{db: db}, nil
}

func newID() string { return modeluuid.New().String() }

func (s *store) subscribe(ctx context.Context, subscriberThreadID modeluuid.UUID, targetThreadID modeluuid.UUID, label string) (bool, error) {
	if subscriberThreadID.IsNull() {
		return false, fmt.Errorf("missing subscriber thread id")
	}
	if targetThreadID.IsNull() {
		return false, fmt.Errorf("missing target thread id")
	}
	label = strings.TrimSpace(label)
	if label == "" {
		label = targetThreadID.String()
	}
	var existing subscriptionRecord
	err := s.db.WithContext(ctx).Where("subscriber_thread_id = ? and target_thread_id = ?", subscriberThreadID.String(), targetThreadID.String()).First(&existing).Error
	if err == nil {
		if strings.TrimSpace(existing.Label) != label {
			existing.Label = label
			if err := s.db.WithContext(ctx).Save(&existing).Error; err != nil {
				return false, err
			}
		}
		return false, nil
	}
	if err != gorm.ErrRecordNotFound {
		return false, err
	}
	record := subscriptionRecord{ID: newID(), SubscriberThreadID: subscriberThreadID.String(), TargetThreadID: targetThreadID.String(), Label: label}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return false, err
	}
	return true, nil
}

func (s *store) unsubscribe(ctx context.Context, subscriberThreadID modeluuid.UUID, targetThreadID modeluuid.UUID) (bool, error) {
	if subscriberThreadID.IsNull() {
		return false, fmt.Errorf("missing subscriber thread id")
	}
	if targetThreadID.IsNull() {
		return false, fmt.Errorf("missing target thread id")
	}
	result := s.db.WithContext(ctx).Where("subscriber_thread_id = ? and target_thread_id = ?", subscriberThreadID.String(), targetThreadID.String()).Delete(&subscriptionRecord{})
	return result.RowsAffected > 0, result.Error
}

func (s *store) subscriptions(ctx context.Context, subscriberThreadID modeluuid.UUID) ([]subscriptionRecord, error) {
	if subscriberThreadID.IsNull() {
		return nil, fmt.Errorf("missing subscriber thread id")
	}
	var records []subscriptionRecord
	if err := s.db.WithContext(ctx).Where("subscriber_thread_id = ?", subscriberThreadID.String()).Order("label asc").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (s *store) subscription(ctx context.Context, subscriberThreadID modeluuid.UUID, targetThreadID modeluuid.UUID) (subscriptionRecord, bool, error) {
	if subscriberThreadID.IsNull() || targetThreadID.IsNull() {
		return subscriptionRecord{}, false, nil
	}
	var record subscriptionRecord
	if err := s.db.WithContext(ctx).Where("subscriber_thread_id = ? and target_thread_id = ?", subscriberThreadID.String(), targetThreadID.String()).First(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return subscriptionRecord{}, false, nil
		}
		return subscriptionRecord{}, false, err
	}
	return record, true, nil
}

func (s *store) markRead(ctx context.Context, subscriberThreadID modeluuid.UUID, targetThreadID modeluuid.UUID, at time.Time) error {
	if subscriberThreadID.IsNull() || targetThreadID.IsNull() || at.IsZero() {
		return nil
	}
	return s.db.WithContext(ctx).Model(&subscriptionRecord{}).
		Where("subscriber_thread_id = ? and target_thread_id = ?", subscriberThreadID.String(), targetThreadID.String()).
		Update("last_read_at", at.UTC()).Error
}
