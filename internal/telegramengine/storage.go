package telegramengine

import (
	"context"

	"github.com/bartdeboer/go-codextgbot/internal/chatmodel"
	"gorm.io/gorm"
)

type UpdateStorage struct {
	DB *gorm.DB
}

func NewUpdateStorage(db *gorm.DB) *UpdateStorage {
	return &UpdateStorage{DB: db}
}

func (s *UpdateStorage) AutoMigrate(ctx context.Context) error {
	return s.DB.WithContext(ctx).AutoMigrate(&chatmodel.TelegramUpdate{})
}

func (s *UpdateStorage) Create(ctx context.Context, upd *chatmodel.TelegramUpdate) error {
	return s.DB.WithContext(ctx).Create(upd).Error
}

func (s *UpdateStorage) Save(ctx context.Context, upd *chatmodel.TelegramUpdate) error {
	return s.DB.WithContext(ctx).Save(upd).Error
}
