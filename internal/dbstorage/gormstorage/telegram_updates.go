package gormstorage

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/dbmodel"
	"github.com/bartdeboer/ctgbot/internal/dbstorage"
	"gorm.io/gorm"
)

type TelegramUpdateStorage struct {
	db *gorm.DB
}

var _ dbstorage.TelegramUpdateStorage = (*TelegramUpdateStorage)(nil)

func (s *TelegramUpdateStorage) Create(ctx context.Context, event *dbmodel.TelegramUpdate) error {
	return s.db.WithContext(ctx).Create(event).Error
}

func (s *TelegramUpdateStorage) Save(ctx context.Context, event *dbmodel.TelegramUpdate) error {
	return s.db.WithContext(ctx).Save(event).Error
}
