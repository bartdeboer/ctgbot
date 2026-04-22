package telegramengine

import (
	"context"

	"gorm.io/gorm"
)

type UpdateStorage struct {
	DB *gorm.DB
}

func NewUpdateStorage(db *gorm.DB) *UpdateStorage {
	return &UpdateStorage{DB: db}
}

func (s *UpdateStorage) AutoMigrate(ctx context.Context) error {
	return s.DB.WithContext(ctx).AutoMigrate(&TelegramUpdate{})
}

func (s *UpdateStorage) Create(ctx context.Context, event *TelegramUpdate) error {
	return s.DB.WithContext(ctx).Create(event).Error
}

func (s *UpdateStorage) Save(ctx context.Context, event *TelegramUpdate) error {
	return s.DB.WithContext(ctx).Save(event).Error
}
