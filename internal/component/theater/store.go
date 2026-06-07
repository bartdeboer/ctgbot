package theater

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const StoreFilename = "theater.db"

type store struct{ db *gorm.DB }

type theaterRecord struct {
	ID        string `gorm:"primaryKey"`
	Name      string `gorm:"uniqueIndex"`
	Label     string
	CreatedAt time.Time
	UpdatedAt time.Time
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

type messageRecord struct {
	ID         string `gorm:"primaryKey"`
	TheaterID  string `gorm:"index"`
	ThreadID   string `gorm:"index"`
	ActorID    string
	ActorLabel string
	Text       string
	CreatedAt  time.Time `gorm:"index"`
}

func (messageRecord) TableName() string { return "theater_messages" }

type pendingUpdate struct {
	TheaterID string
	Name      string
	Label     string
	Count     int64
}

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
	if err := db.AutoMigrate(&theaterRecord{}, &subscriptionRecord{}, &messageRecord{}); err != nil {
		return nil, fmt.Errorf("migrate theater db: %w", err)
	}
	return &store{db: db}, nil
}

func newID() string { return modeluuid.New().String() }

func normalizeName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (s *store) createTheater(ctx context.Context, name string) (theaterRecord, bool, error) {
	if s == nil || s.db == nil {
		return theaterRecord{}, false, fmt.Errorf("missing theater store")
	}
	name = normalizeName(name)
	if name == "" {
		return theaterRecord{}, false, fmt.Errorf("missing theater name")
	}
	var existing theaterRecord
	if err := s.db.WithContext(ctx).Where("name = ?", name).First(&existing).Error; err == nil {
		return existing, false, nil
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return theaterRecord{}, false, err
	}
	record := theaterRecord{ID: newID(), Name: name, Label: name}
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

func (s *store) post(ctx context.Context, theater theaterRecord, threadID modeluuid.UUID, actorID string, actorLabel string, text string) (messageRecord, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return messageRecord{}, fmt.Errorf("missing theater message")
	}
	record := messageRecord{ID: newID(), TheaterID: theater.ID, ThreadID: threadID.String(), ActorID: strings.TrimSpace(actorID), ActorLabel: strings.TrimSpace(actorLabel), Text: text, CreatedAt: time.Now().UTC()}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return messageRecord{}, err
	}
	return record, nil
}

func (s *store) read(ctx context.Context, theater theaterRecord, threadID modeluuid.UUID, limit int) ([]messageRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	var desc []messageRecord
	if err := s.db.WithContext(ctx).Where("theater_id = ?", theater.ID).Order("created_at desc").Limit(limit).Find(&desc).Error; err != nil {
		return nil, err
	}
	records := make([]messageRecord, len(desc))
	for i := range desc {
		records[len(desc)-1-i] = desc[i]
	}
	if len(records) > 0 && !threadID.IsNull() {
		last := records[len(records)-1].CreatedAt.UTC()
		_ = s.db.WithContext(ctx).Model(&subscriptionRecord{}).Where("theater_id = ? and thread_id = ?", theater.ID, threadID.String()).Update("last_read_at", last).Error
	}
	return records, nil
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

func (s *store) pendingUpdates(ctx context.Context, threadID modeluuid.UUID) ([]component.UpdateNotice, error) {
	if threadID.IsNull() {
		return nil, nil
	}
	var rows []pendingUpdate
	query := `
SELECT t.id AS theater_id, t.name, t.label, COUNT(m.id) AS count
FROM theater_subscriptions s
JOIN theaters t ON t.id = s.theater_id
JOIN theater_messages m ON m.theater_id = t.id
WHERE s.thread_id = ?
  AND (s.last_read_at IS NULL OR m.created_at > s.last_read_at)
GROUP BY t.id, t.name, t.label
ORDER BY t.name ASC`
	if err := s.db.WithContext(ctx).Raw(query, threadID.String()).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]component.UpdateNotice, 0, len(rows))
	for _, row := range rows {
		if row.Count <= 0 {
			continue
		}
		label := strings.TrimSpace(row.Label)
		if label == "" {
			label = row.Name
		}
		out = append(out, component.UpdateNotice{Source: Type, Ref: row.Name, Label: label, Kind: "message", Count: int(row.Count)})
	}
	return out, nil
}

func (s *store) pendingCount(ctx context.Context, theaterID string, threadID modeluuid.UUID) (int64, error) {
	var subscription subscriptionRecord
	if err := s.db.WithContext(ctx).Where("theater_id = ? and thread_id = ?", theaterID, threadID.String()).First(&subscription).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, nil
		}
		return 0, err
	}
	query := s.db.WithContext(ctx).Model(&messageRecord{}).Where("theater_id = ?", theaterID)
	if subscription.LastReadAt != nil {
		query = query.Where("created_at > ?", subscription.LastReadAt.UTC())
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
