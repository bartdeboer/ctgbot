package gormstorage

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/dbmodel"
	"github.com/bartdeboer/ctgbot/internal/dbstorage"
	"gorm.io/gorm"
)

type Storage struct {
	db              *gorm.DB
	threads         dbstorage.ThreadStorage
	telegramUpdates dbstorage.TelegramUpdateStorage
}

var _ dbstorage.Storage = (*Storage)(nil)

func New(db *gorm.DB) *Storage {
	return &Storage{
		db:              db,
		threads:         &ThreadStorage{db: db},
		telegramUpdates: &TelegramUpdateStorage{db: db},
	}
}

func (s *Storage) AutoMigrate(ctx context.Context) error {
	return s.db.WithContext(ctx).AutoMigrate(
		&dbmodel.Thread{},
		&dbmodel.TelegramUpdate{},
	)
}

func (s *Storage) Threads() dbstorage.ThreadStorage {
	if s == nil {
		return nil
	}
	return s.threads
}

func (s *Storage) TelegramUpdates() dbstorage.TelegramUpdateStorage {
	if s == nil {
		return nil
	}
	return s.telegramUpdates
}
