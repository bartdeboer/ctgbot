package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
	"gorm.io/gorm"
)

type GORMStorage struct {
	db                *gorm.DB
	chats             *GORMChats
	threads           *GORMThreads
	components        *GORMComponents
	componentProfiles *GORMComponentProfiles
	chatComponents    *GORMChatComponents
	threadStates      *GORMThreadComponentStates
	messages          *GORMMessages
	artifacts         *GORMArtifacts
}

var _ Storage = (*GORMStorage)(nil)

func NewGORM(db *gorm.DB) *GORMStorage {
	return &GORMStorage{
		db:                db,
		chats:             &GORMChats{db: db},
		threads:           &GORMThreads{db: db},
		components:        &GORMComponents{db: db},
		componentProfiles: &GORMComponentProfiles{db: db},
		chatComponents:    &GORMChatComponents{db: db},
		threadStates:      &GORMThreadComponentStates{db: db},
		messages:          &GORMMessages{db: db},
		artifacts:         &GORMArtifacts{db: db},
	}
}

func (s *GORMStorage) AutoMigrate(ctx context.Context) error {
	return s.db.WithContext(ctx).AutoMigrate(
		&coremodel.Chat{},
		&coremodel.Thread{},
		&coremodel.Component{},
		&coremodel.ComponentProfile{},
		&coremodel.ChatComponent{},
		&coremodel.ThreadComponentState{},
		&coremodel.ThreadMessage{},
		&coremodel.Artifact{},
	)
}

func (s *GORMStorage) Transaction(ctx context.Context, fn func(Storage) error) error {
	if fn == nil {
		return fmt.Errorf("missing transaction function")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(NewGORM(tx))
	})
}

func (s *GORMStorage) Chats() ChatRepository                         { return s.chats }
func (s *GORMStorage) Threads() ThreadRepository                     { return s.threads }
func (s *GORMStorage) Components() ComponentRepository               { return s.components }
func (s *GORMStorage) ComponentProfiles() ComponentProfileRepository { return s.componentProfiles }
func (s *GORMStorage) ChatComponents() ChatComponentRepository       { return s.chatComponents }
func (s *GORMStorage) ThreadComponentStates() ThreadComponentStateRepository {
	return s.threadStates
}
func (s *GORMStorage) Messages() MessageRepository   { return s.messages }
func (s *GORMStorage) Artifacts() ArtifactRepository { return s.artifacts }

type GORMChats struct{ db *gorm.DB }

var _ ChatRepository = (*GORMChats)(nil)

func (r *GORMChats) Save(ctx context.Context, chat *coremodel.Chat) error {
	ensureID(&chat.ID)
	chat.ProviderType = clean(chat.ProviderType)
	chat.ProviderChatID = clean(chat.ProviderChatID)
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

func (r *GORMChats) EnsureProviderChat(ctx context.Context, providerType string, providerChatID string) (*coremodel.Chat, error) {
	providerType = clean(providerType)
	providerChatID = clean(providerChatID)
	if providerType == "" {
		return nil, fmt.Errorf("missing provider type")
	}
	if providerChatID == "" {
		return nil, fmt.Errorf("missing provider chat id")
	}

	var chat coremodel.Chat
	err := first(r.db.WithContext(ctx).
		Where("provider_type = ? AND provider_chat_id = ?", providerType, providerChatID).
		First(&chat))
	if err != nil {
		return nil, err
	}
	if !chat.ID.IsNull() {
		return &chat, nil
	}

	chat = coremodel.Chat{
		ID:             modeluuid.New(),
		ProviderType:   providerType,
		ProviderChatID: providerChatID,
	}
	if err := r.db.WithContext(ctx).Create(&chat).Error; err != nil {
		return nil, err
	}
	return &chat, nil
}

func (r *GORMChats) ListDisabled(ctx context.Context) ([]coremodel.Chat, error) {
	var chats []coremodel.Chat
	err := r.db.WithContext(ctx).
		Where("enabled = ?", false).
		Order("created_at ASC").
		Find(&chats).Error
	return chats, err
}

type GORMThreads struct{ db *gorm.DB }

var _ ThreadRepository = (*GORMThreads)(nil)

func (r *GORMThreads) Save(ctx context.Context, thread *coremodel.Thread) error {
	ensureID(&thread.ID)
	thread.ProviderThreadID = clean(thread.ProviderThreadID)
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

func (r *GORMThreads) EnsureProviderThread(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*coremodel.Thread, error) {
	providerThreadID = clean(providerThreadID)
	if chatID.IsNull() {
		return nil, fmt.Errorf("missing chat id")
	}
	if providerThreadID == "" {
		providerThreadID = "0"
	}

	var thread coremodel.Thread
	err := first(r.db.WithContext(ctx).
		Where("chat_id = ? AND provider_thread_id = ?", chatID, providerThreadID).
		First(&thread))
	if err != nil {
		return nil, err
	}
	if !thread.ID.IsNull() {
		return &thread, nil
	}

	thread = coremodel.Thread{
		ID:               modeluuid.New(),
		ChatID:           chatID,
		ProviderThreadID: providerThreadID,
	}
	if err := r.db.WithContext(ctx).Create(&thread).Error; err != nil {
		return nil, err
	}
	return &thread, nil
}

func (r *GORMThreads) ListByChatID(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.Thread, error) {
	var threads []coremodel.Thread
	err := r.db.WithContext(ctx).Where("chat_id = ?", chatID).Order("created_at ASC").Find(&threads).Error
	return threads, err
}

type GORMComponents struct{ db *gorm.DB }

var _ ComponentRepository = (*GORMComponents)(nil)

func (r *GORMComponents) Save(ctx context.Context, component *coremodel.Component) error {
	component.Type = clean(component.Type)
	if component.ID.IsNull() {
		existing, err := r.GetByType(ctx, component.Type)
		if err != nil {
			return err
		}
		if existing != nil {
			component.ID = existing.ID
		}
	}
	ensureID(&component.ID)
	return r.db.WithContext(ctx).Save(component).Error
}

func (r *GORMComponents) GetByType(ctx context.Context, componentType string) (*coremodel.Component, error) {
	var component coremodel.Component
	if err := first(r.db.WithContext(ctx).Where("type = ?", clean(componentType)).First(&component)); err != nil {
		return nil, err
	}
	if component.ID.IsNull() {
		return nil, nil
	}
	return &component, nil
}

type GORMComponentProfiles struct{ db *gorm.DB }

var _ ComponentProfileRepository = (*GORMComponentProfiles)(nil)

func (r *GORMComponentProfiles) Save(ctx context.Context, profile *coremodel.ComponentProfile) error {
	profile.ComponentType = clean(profile.ComponentType)
	profile.ProfileName = clean(profile.ProfileName)
	if profile.ID.IsNull() {
		existing, err := r.Get(ctx, profile.ComponentType, profile.ProfileName)
		if err != nil {
			return err
		}
		if existing != nil {
			profile.ID = existing.ID
		}
	}
	ensureID(&profile.ID)
	return r.db.WithContext(ctx).Save(profile).Error
}

func (r *GORMComponentProfiles) Get(ctx context.Context, componentType string, profileName string) (*coremodel.ComponentProfile, error) {
	var profile coremodel.ComponentProfile
	if err := first(r.db.WithContext(ctx).
		Where("component_type = ? AND profile_name = ?", clean(componentType), clean(profileName)).
		First(&profile)); err != nil {
		return nil, err
	}
	if profile.ID.IsNull() {
		return nil, nil
	}
	return &profile, nil
}

type GORMChatComponents struct{ db *gorm.DB }

var _ ChatComponentRepository = (*GORMChatComponents)(nil)

func (r *GORMChatComponents) Save(ctx context.Context, binding *coremodel.ChatComponent) error {
	binding.ComponentType = clean(binding.ComponentType)
	binding.ProfileName = clean(binding.ProfileName)
	if binding.ID.IsNull() {
		existing, err := r.get(ctx, binding.ChatID, binding.ComponentType, binding.ProfileName)
		if err != nil {
			return err
		}
		if existing != nil {
			binding.ID = existing.ID
		}
	}
	ensureID(&binding.ID)
	return r.db.WithContext(ctx).Save(binding).Error
}

func (r *GORMChatComponents) get(ctx context.Context, chatID modeluuid.UUID, componentType string, profileName string) (*coremodel.ChatComponent, error) {
	var binding coremodel.ChatComponent
	if err := first(r.db.WithContext(ctx).
		Where("chat_id = ? AND component_type = ? AND profile_name = ?", chatID, clean(componentType), clean(profileName)).
		First(&binding)); err != nil {
		return nil, err
	}
	if binding.ID.IsNull() {
		return nil, nil
	}
	return &binding, nil
}

func (r *GORMChatComponents) ListByChatID(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.ChatComponent, error) {
	return r.listByChatID(ctx, chatID, false)
}

func (r *GORMChatComponents) ListEnabledByChatID(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.ChatComponent, error) {
	return r.listByChatID(ctx, chatID, true)
}

func (r *GORMChatComponents) listByChatID(ctx context.Context, chatID modeluuid.UUID, enabledOnly bool) ([]coremodel.ChatComponent, error) {
	var bindings []coremodel.ChatComponent
	tx := r.db.WithContext(ctx).Where("chat_id = ?", chatID)
	if enabledOnly {
		tx = tx.Where("enabled = ?", true)
	}
	err := tx.Order("created_at ASC").Find(&bindings).Error
	return bindings, err
}

type GORMThreadComponentStates struct{ db *gorm.DB }

var _ ThreadComponentStateRepository = (*GORMThreadComponentStates)(nil)

func (r *GORMThreadComponentStates) Save(ctx context.Context, state *coremodel.ThreadComponentState) error {
	state.ComponentType = clean(state.ComponentType)
	state.ProfileName = clean(state.ProfileName)
	if state.ID.IsNull() {
		existing, err := r.Get(ctx, state.ThreadID, state.ComponentType, state.ProfileName)
		if err != nil {
			return err
		}
		if existing != nil {
			state.ID = existing.ID
		}
	}
	ensureID(&state.ID)
	return r.db.WithContext(ctx).Save(state).Error
}

func (r *GORMThreadComponentStates) Get(ctx context.Context, threadID modeluuid.UUID, componentType string, profileName string) (*coremodel.ThreadComponentState, error) {
	var state coremodel.ThreadComponentState
	if err := first(r.db.WithContext(ctx).
		Where("thread_id = ? AND component_type = ? AND profile_name = ?", threadID, clean(componentType), clean(profileName)).
		First(&state)); err != nil {
		return nil, err
	}
	if state.ID.IsNull() {
		return nil, nil
	}
	return &state, nil
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

func clean(value string) string {
	return strings.TrimSpace(value)
}

func first(tx *gorm.DB) error {
	if tx.Error == gorm.ErrRecordNotFound {
		return nil
	}
	return tx.Error
}
