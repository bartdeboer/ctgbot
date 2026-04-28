package repository

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
	"gorm.io/gorm"
)

type GORMStorage struct {
	db        *gorm.DB
	chats     *GORMChats
	threads   *GORMThreads
	messages  *GORMMessages
	artifacts *GORMArtifacts
}

var _ Storage = (*GORMStorage)(nil)

func NewGORM(db *gorm.DB) *GORMStorage {
	return &GORMStorage{
		db:        db,
		chats:     &GORMChats{db: db},
		threads:   &GORMThreads{db: db},
		messages:  &GORMMessages{db: db},
		artifacts: &GORMArtifacts{db: db},
	}
}

func (s *GORMStorage) AutoMigrate(ctx context.Context) error {
	return s.db.WithContext(ctx).AutoMigrate(
		&coremodel.Chat{},
		&coremodel.Thread{},
		&coremodel.ThreadMessage{},
		&coremodel.Artifact{},
	)
}

func (s *GORMStorage) Chats() ChatRepository         { return s.chats }
func (s *GORMStorage) Threads() ThreadRepository     { return s.threads }
func (s *GORMStorage) Messages() MessageRepository   { return s.messages }
func (s *GORMStorage) Artifacts() ArtifactRepository { return s.artifacts }

type GORMChats struct{ db *gorm.DB }

var _ ChatRepository = (*GORMChats)(nil)

func (r *GORMChats) Save(ctx context.Context, chat *coremodel.Chat) error {
	ensureID(&chat.ID)
	return r.db.WithContext(ctx).Save(chat).Error
}

func (r *GORMChats) GetByID(ctx context.Context, chatID modeluuid.UUID) (*coremodel.Chat, error) {
	var chat coremodel.Chat
	if err := first(r.db.WithContext(ctx).Where("id = ?", chatID).First(&chat)); err != nil {
		return nil, err
	}
	if chat.ID.IsNull() {
		return nil, nil
	}
	return &chat, nil
}

type GORMThreads struct{ db *gorm.DB }

var _ ThreadRepository = (*GORMThreads)(nil)

func (r *GORMThreads) Save(ctx context.Context, thread *coremodel.Thread) error {
	ensureID(&thread.ID)
	return r.db.WithContext(ctx).Save(thread).Error
}

func (r *GORMThreads) GetByID(ctx context.Context, threadID modeluuid.UUID) (*coremodel.Thread, error) {
	var thread coremodel.Thread
	if err := first(r.db.WithContext(ctx).Where("id = ?", threadID).First(&thread)); err != nil {
		return nil, err
	}
	if thread.ID.IsNull() {
		return nil, nil
	}
	return &thread, nil
}

func (r *GORMThreads) ListByChatID(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.Thread, error) {
	var threads []coremodel.Thread
	err := r.db.WithContext(ctx).Where("chat_id = ?", chatID).Order("created_at ASC").Find(&threads).Error
	return threads, err
}

type GORMMessages struct{ db *gorm.DB }

var _ MessageRepository = (*GORMMessages)(nil)

func (r *GORMMessages) Append(ctx context.Context, message *coremodel.ThreadMessage) error {
	ensureID(&message.ID)
	return r.db.WithContext(ctx).Create(message).Error
}

func (r *GORMMessages) GetByID(ctx context.Context, messageID modeluuid.UUID) (*coremodel.ThreadMessage, error) {
	var message coremodel.ThreadMessage
	if err := first(r.db.WithContext(ctx).Where("id = ?", messageID).First(&message)); err != nil {
		return nil, err
	}
	if message.ID.IsNull() {
		return nil, nil
	}
	return &message, nil
}

func (r *GORMMessages) ListByThreadID(ctx context.Context, threadID modeluuid.UUID) ([]coremodel.ThreadMessage, error) {
	var messages []coremodel.ThreadMessage
	err := r.db.WithContext(ctx).Where("thread_id = ?", threadID).Order("created_at ASC").Find(&messages).Error
	return messages, err
}

type GORMArtifacts struct{ db *gorm.DB }

var _ ArtifactRepository = (*GORMArtifacts)(nil)

func (r *GORMArtifacts) Append(ctx context.Context, artifact *coremodel.Artifact) error {
	ensureID(&artifact.ID)
	return r.db.WithContext(ctx).Create(artifact).Error
}

func (r *GORMArtifacts) ListByMessageID(ctx context.Context, messageID modeluuid.UUID) ([]coremodel.Artifact, error) {
	var artifacts []coremodel.Artifact
	err := r.db.WithContext(ctx).Where("message_id = ?", messageID).Order("created_at ASC").Find(&artifacts).Error
	return artifacts, err
}

func ensureID(id *modeluuid.UUID) {
	if id != nil && id.IsNull() {
		*id = modeluuid.New()
	}
}

func first(tx *gorm.DB) error {
	if tx.Error == gorm.ErrRecordNotFound {
		return nil
	}
	return tx.Error
}
