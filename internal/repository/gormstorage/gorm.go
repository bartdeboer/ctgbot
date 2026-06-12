package gormstorage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	"gorm.io/gorm"
)

type GORMStorage struct {
	artifactDir             string
	db                      *gorm.DB
	chats                   *gormChats
	threads                 *gormThreads
	components              *gormComponents
	chatComponents          *gormChatComponents
	inboundFilterBindings   *gormInboundFilterBindings
	inboundDrops            *gormInboundDrops
	droppedEvents           *gormDroppedEvents
	allowlistSenders        *gormAllowlistSenders
	threadComponentMappings *gormThreadComponentMappings
	threadComponentStates   *gormThreadComponentStates
	messages                *gormMessages
	artifacts               *gormArtifacts
	scheduledJobs           *gormScheduledJobs
	timedIntents            *gormTimedIntents
	trustedControllers      *gormTrustedControllers
}

func New(db *gorm.DB) *GORMStorage {
	return NewWithArtifactDir(db, "")
}

func NewWithArtifactDir(db *gorm.DB, artifactDir string) *GORMStorage {
	artifactDir = clean(artifactDir)
	return &GORMStorage{
		artifactDir:             artifactDir,
		db:                      db,
		chats:                   &gormChats{db: db},
		threads:                 &gormThreads{db: db},
		components:              &gormComponents{db: db},
		chatComponents:          &gormChatComponents{db: db},
		inboundFilterBindings:   &gormInboundFilterBindings{db: db},
		inboundDrops:            &gormInboundDrops{db: db},
		droppedEvents:           &gormDroppedEvents{db: db},
		allowlistSenders:        &gormAllowlistSenders{db: db},
		threadComponentMappings: &gormThreadComponentMappings{db: db},
		threadComponentStates:   &gormThreadComponentStates{db: db},
		messages:                &gormMessages{db: db},
		artifacts:               &gormArtifacts{db: db, artifactDir: artifactDir},
		scheduledJobs:           &gormScheduledJobs{db: db},
		timedIntents:            &gormTimedIntents{db: db},
		trustedControllers:      &gormTrustedControllers{db: db},
	}
}

func (s *GORMStorage) AutoMigrate(ctx context.Context) error {
	if err := s.migrateProviderChannelColumns(ctx); err != nil {
		return err
	}
	if err := s.migrateComponentProfileColumn(ctx); err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).AutoMigrate(
		&coremodel.Chat{},
		&coremodel.Thread{},
		&coremodel.Component{},
		&coremodel.ChatComponent{},
		&coremodel.InboundFilterBinding{},
		&coremodel.InboundDrop{},
		&coremodel.DroppedEvent{},
		&coremodel.AllowlistSender{},
		&coremodel.ThreadComponentMapping{},
		&coremodel.ThreadComponentState{},
		&coremodel.ThreadMessage{},
		&coremodel.Artifact{},
		&coremodel.ScheduledJob{},
		&coremodel.TimedIntent{},
		&coremodel.TrustedController{},
	); err != nil {
		return err
	}
	return nil
}

func (s *GORMStorage) Transaction(ctx context.Context, fn func(repository.Storage) error) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("missing db")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(NewWithArtifactDir(tx, s.artifactDir))
	})
}

func (s *GORMStorage) Chats() repository.ChatRepository                   { return s.chats }
func (s *GORMStorage) Threads() repository.ThreadRepository               { return s.threads }
func (s *GORMStorage) Components() repository.ComponentRepository         { return s.components }
func (s *GORMStorage) ChatComponents() repository.ChatComponentRepository { return s.chatComponents }
func (s *GORMStorage) InboundFilterBindings() repository.InboundFilterBindingRepository {
	return s.inboundFilterBindings
}
func (s *GORMStorage) InboundDrops() repository.InboundDropRepository   { return s.inboundDrops }
func (s *GORMStorage) DroppedEvents() repository.DroppedEventRepository { return s.droppedEvents }
func (s *GORMStorage) AllowlistSenders() repository.AllowlistSenderRepository {
	return s.allowlistSenders
}
func (s *GORMStorage) ThreadComponentMappings() repository.ThreadComponentMappingRepository {
	return s.threadComponentMappings
}
func (s *GORMStorage) ThreadComponentStates() repository.ThreadComponentStateRepository {
	return s.threadComponentStates
}
func (s *GORMStorage) Messages() repository.MessageRepository   { return s.messages }
func (s *GORMStorage) Artifacts() repository.ArtifactRepository { return s.artifacts }
func (s *GORMStorage) ScheduledJobs() repository.ScheduledJobRepository {
	return s.scheduledJobs
}

func (s *GORMStorage) TimedIntents() repository.TimedIntentRepository {
	return s.timedIntents
}

func (s *GORMStorage) TrustedControllers() repository.TrustedControllerRepository {
	return s.trustedControllers
}

type gormChats struct{ db *gorm.DB }

func (r *gormChats) Save(ctx context.Context, chat *coremodel.Chat) error {
	chat.Label = strings.TrimSpace(chat.Label)
	chat.Workspace = clean(chat.Workspace)
	ensureID(&chat.ID)
	return r.db.WithContext(ctx).Save(chat).Error
}

func (r *gormChats) GetByID(ctx context.Context, chatID modeluuid.UUID) (*coremodel.Chat, error) {
	var chat coremodel.Chat
	if err := first(r.db.WithContext(ctx).Where("id = ?", chatID).First(&chat)); err != nil {
		return nil, err
	}
	if chat.ID.IsNull() {
		return nil, nil
	}
	return &chat, nil
}

func (r *gormChats) List(ctx context.Context) ([]coremodel.Chat, error) {
	var chats []coremodel.Chat
	err := r.db.WithContext(ctx).Order("created_at ASC").Find(&chats).Error
	return chats, err
}

func (r *gormChats) ListIDs(ctx context.Context) ([]modeluuid.UUID, error) {
	var chats []coremodel.Chat
	if err := r.db.WithContext(ctx).Select("id").Find(&chats).Error; err != nil {
		return nil, err
	}
	ids := make([]modeluuid.UUID, 0, len(chats))
	for _, chat := range chats {
		if chat.ID.IsNull() {
			continue
		}
		ids = append(ids, chat.ID)
	}
	return ids, nil
}

type gormThreads struct{ db *gorm.DB }

func (r *gormThreads) Save(ctx context.Context, thread *coremodel.Thread) error {
	thread.Label = strings.TrimSpace(thread.Label)
	thread.VoiceLanguage = clean(thread.VoiceLanguage)
	thread.VoiceName = clean(thread.VoiceName)
	thread.VoiceModel = clean(thread.VoiceModel)
	thread.VoiceDeviceTarget = clean(thread.VoiceDeviceTarget)
	ensureID(&thread.ID)
	return r.db.WithContext(ctx).Save(thread).Error
}

func (r *gormThreads) GetByID(ctx context.Context, threadID modeluuid.UUID) (*coremodel.Thread, error) {
	var thread coremodel.Thread
	if err := first(r.db.WithContext(ctx).Where("id = ?", threadID).First(&thread)); err != nil {
		return nil, err
	}
	if thread.ID.IsNull() {
		return nil, nil
	}
	return &thread, nil
}

func (r *gormThreads) ListByChatID(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.Thread, error) {
	var threads []coremodel.Thread
	err := r.db.WithContext(ctx).Where("chat_id = ?", chatID).Order("created_at ASC").Find(&threads).Error
	return threads, err
}

func (r *gormThreads) ListIDs(ctx context.Context) ([]modeluuid.UUID, error) {
	var threads []coremodel.Thread
	if err := r.db.WithContext(ctx).Select("id").Find(&threads).Error; err != nil {
		return nil, err
	}
	ids := make([]modeluuid.UUID, 0, len(threads))
	for _, thread := range threads {
		if thread.ID.IsNull() {
			continue
		}
		ids = append(ids, thread.ID)
	}
	return ids, nil
}

type gormComponents struct{ db *gorm.DB }

func (r *gormComponents) Save(ctx context.Context, component *coremodel.Component) error {
	component.Type = clean(component.Type)
	component.Name = clean(component.Name)
	component.Runtime = clean(component.Runtime)
	component.ProfilePath = clean(component.ProfilePath)
	component.Label = strings.TrimSpace(component.Label)
	if component.ID.IsNull() {
		existing, err := r.GetByTypeAndName(ctx, component.Type, component.Name)
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

func (r *gormComponents) GetByID(ctx context.Context, componentID modeluuid.UUID) (*coremodel.Component, error) {
	var componentRow coremodel.Component
	if err := first(r.db.WithContext(ctx).Where("id = ?", componentID).First(&componentRow)); err != nil {
		return nil, err
	}
	if componentRow.ID.IsNull() {
		return nil, nil
	}
	return &componentRow, nil
}

func (r *gormComponents) GetDefaultByType(ctx context.Context, componentType string) (*coremodel.Component, error) {
	var componentRow coremodel.Component
	if err := first(r.db.WithContext(ctx).
		Where("type = ? AND is_default = ? AND enabled = ?", clean(componentType), true, true).
		First(&componentRow)); err != nil {
		return nil, err
	}
	if componentRow.ID.IsNull() {
		return nil, nil
	}
	return &componentRow, nil
}

func (r *gormComponents) GetByTypeAndName(ctx context.Context, componentType string, name string) (*coremodel.Component, error) {
	var componentRow coremodel.Component
	if err := first(r.db.WithContext(ctx).
		Where("type = ? AND name = ?", clean(componentType), clean(name)).
		First(&componentRow)); err != nil {
		return nil, err
	}
	if componentRow.ID.IsNull() {
		return nil, nil
	}
	return &componentRow, nil
}

func (r *gormComponents) ListEnabled(ctx context.Context) ([]coremodel.Component, error) {
	var out []coremodel.Component
	err := r.db.WithContext(ctx).Where("enabled = ?", true).Order("created_at ASC").Find(&out).Error
	return out, err
}

func (r *gormComponents) DeleteByID(ctx context.Context, componentID modeluuid.UUID) (bool, error) {
	result := r.db.WithContext(ctx).
		Where("id = ?", componentID).
		Delete(&coremodel.Component{})
	return result.RowsAffected > 0, result.Error
}

type gormChatComponents struct{ db *gorm.DB }

func (r *gormChatComponents) Save(ctx context.Context, binding *coremodel.ChatComponent) error {
	binding.ExternalChannelID = clean(binding.ExternalChannelID)
	if binding.ID.IsNull() {
		existing, err := r.GetByChatComponentRole(ctx, binding.ChatID, binding.ComponentID, binding.Role)
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

func (r *gormChatComponents) GetByChatComponentRole(ctx context.Context, chatID modeluuid.UUID, componentID modeluuid.UUID, role coremodel.ChatComponentRole) (*coremodel.ChatComponent, error) {
	var binding coremodel.ChatComponent
	if err := first(r.db.WithContext(ctx).
		Where("chat_id = ? AND component_id = ? AND role = ?", chatID, componentID, role).
		First(&binding)); err != nil {
		return nil, err
	}
	if binding.ID.IsNull() {
		return nil, nil
	}
	return &binding, nil
}

func (r *gormChatComponents) ListEnabledByChatID(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.ChatComponent, error) {
	var bindings []coremodel.ChatComponent
	err := r.db.WithContext(ctx).
		Where("chat_id = ? AND enabled = ?", chatID, true).
		Order("created_at ASC").
		Find(&bindings).Error
	return bindings, err
}

func (r *gormChatComponents) ListByComponentID(ctx context.Context, componentID modeluuid.UUID) ([]coremodel.ChatComponent, error) {
	var bindings []coremodel.ChatComponent
	err := r.db.WithContext(ctx).
		Where("component_id = ?", componentID).
		Order("created_at ASC").
		Find(&bindings).Error
	return bindings, err
}

func (r *gormChatComponents) FindByComponentRoleAndExternalChannelID(ctx context.Context, componentID modeluuid.UUID, role coremodel.ChatComponentRole, externalChannelID string) (*coremodel.ChatComponent, error) {
	var binding coremodel.ChatComponent
	if err := first(r.db.WithContext(ctx).
		Where("component_id = ? AND role = ? AND external_channel_id = ? AND enabled = ?", componentID, role, clean(externalChannelID), true).
		First(&binding)); err != nil {
		return nil, err
	}
	if binding.ID.IsNull() {
		return nil, nil
	}
	return &binding, nil
}

func (r *gormChatComponents) DeleteByComponentID(ctx context.Context, componentID modeluuid.UUID) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("component_id = ?", componentID).
		Delete(&coremodel.ChatComponent{})
	return result.RowsAffected, result.Error
}

type gormInboundFilterBindings struct{ db *gorm.DB }

func (r *gormInboundFilterBindings) Save(ctx context.Context, binding *coremodel.InboundFilterBinding) error {
	if binding.ID.IsNull() {
		existing, err := r.GetBySourceBindingAndFilter(ctx, binding.SourceBindingID, binding.FilterComponentID)
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

func (r *gormInboundFilterBindings) GetBySourceBindingAndFilter(ctx context.Context, sourceBindingID modeluuid.UUID, filterComponentID modeluuid.UUID) (*coremodel.InboundFilterBinding, error) {
	var binding coremodel.InboundFilterBinding
	if err := first(r.db.WithContext(ctx).
		Where("source_binding_id = ? AND filter_component_id = ?", sourceBindingID, filterComponentID).
		First(&binding)); err != nil {
		return nil, err
	}
	if binding.ID.IsNull() {
		return nil, nil
	}
	return &binding, nil
}

func (r *gormInboundFilterBindings) ListEnabledBySourceBindingID(ctx context.Context, sourceBindingID modeluuid.UUID) ([]coremodel.InboundFilterBinding, error) {
	var bindings []coremodel.InboundFilterBinding
	err := r.db.WithContext(ctx).
		Where("source_binding_id = ? AND enabled = ?", sourceBindingID, true).
		Order("created_at ASC").
		Find(&bindings).Error
	return bindings, err
}

func (r *gormInboundFilterBindings) DeleteByFilterComponentID(ctx context.Context, componentID modeluuid.UUID) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("filter_component_id = ?", componentID).
		Delete(&coremodel.InboundFilterBinding{})
	return result.RowsAffected, result.Error
}

func (r *gormInboundFilterBindings) DeleteBySourceBindingIDs(ctx context.Context, sourceBindingIDs []modeluuid.UUID) (int64, error) {
	if len(sourceBindingIDs) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).
		Where("source_binding_id IN ?", sourceBindingIDs).
		Delete(&coremodel.InboundFilterBinding{})
	return result.RowsAffected, result.Error
}

type gormInboundDrops struct{ db *gorm.DB }

func (r *gormInboundDrops) Save(ctx context.Context, drop *coremodel.InboundDrop) error {
	drop.ExternalChannelID = clean(drop.ExternalChannelID)
	drop.ExternalThreadID = clean(drop.ExternalThreadID)
	drop.ChatLabel = strings.TrimSpace(drop.ChatLabel)
	drop.ActorID = clean(drop.ActorID)
	drop.ActorLabel = strings.TrimSpace(drop.ActorLabel)
	drop.LastTextPreview = strings.TrimSpace(drop.LastTextPreview)
	if drop.ID.IsNull() {
		existing, err := r.GetByComponentAndExternalChannelID(ctx, drop.ComponentID, drop.ExternalChannelID)
		if err != nil {
			return err
		}
		if existing != nil {
			drop.ID = existing.ID
			if drop.FirstSeenAt.IsZero() {
				drop.FirstSeenAt = existing.FirstSeenAt
			}
		}
	}
	ensureID(&drop.ID)
	return r.db.WithContext(ctx).Save(drop).Error
}

func (r *gormInboundDrops) GetByComponentAndExternalChannelID(ctx context.Context, componentID modeluuid.UUID, externalChannelID string) (*coremodel.InboundDrop, error) {
	var drop coremodel.InboundDrop
	if err := first(r.db.WithContext(ctx).
		Where("component_id = ? AND external_channel_id = ?", componentID, clean(externalChannelID)).
		First(&drop)); err != nil {
		return nil, err
	}
	if drop.ID.IsNull() {
		return nil, nil
	}
	return &drop, nil
}

func (r *gormInboundDrops) List(ctx context.Context) ([]coremodel.InboundDrop, error) {
	var drops []coremodel.InboundDrop
	err := r.db.WithContext(ctx).
		Order("last_seen_at DESC").
		Order("first_seen_at DESC").
		Find(&drops).Error
	return drops, err
}

func (r *gormInboundDrops) DeleteByComponentAndExternalChannelID(ctx context.Context, componentID modeluuid.UUID, externalChannelID string) error {
	return r.db.WithContext(ctx).
		Where("component_id = ? AND external_channel_id = ?", componentID, clean(externalChannelID)).
		Delete(&coremodel.InboundDrop{}).
		Error
}

type gormDroppedEvents struct{ db *gorm.DB }

func (r *gormDroppedEvents) Save(ctx context.Context, event *coremodel.DroppedEvent) error {
	event.ProviderChannelID = clean(event.ProviderChannelID)
	event.ProviderThreadID = clean(event.ProviderThreadID)
	event.ProviderMessageID = clean(event.ProviderMessageID)
	event.SenderKey = clean(event.SenderKey)
	event.SenderLabel = strings.TrimSpace(event.SenderLabel)
	event.Subject = strings.TrimSpace(event.Subject)
	event.Preview = strings.TrimSpace(event.Preview)
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	if event.ExpiresAt.IsZero() {
		event.ExpiresAt = event.CreatedAt.Add(30 * 24 * time.Hour)
	}
	ensureID(&event.ID)
	return r.db.WithContext(ctx).Save(event).Error
}

func (r *gormDroppedEvents) GetByID(ctx context.Context, eventID modeluuid.UUID) (*coremodel.DroppedEvent, error) {
	var event coremodel.DroppedEvent
	if err := first(r.db.WithContext(ctx).Where("id = ?", eventID).First(&event)); err != nil {
		return nil, err
	}
	if event.ID.IsNull() {
		return nil, nil
	}
	return &event, nil
}

func (r *gormDroppedEvents) ListIDs(ctx context.Context) ([]modeluuid.UUID, error) {
	var events []coremodel.DroppedEvent
	if err := r.db.WithContext(ctx).Select("id").Find(&events).Error; err != nil {
		return nil, err
	}
	ids := make([]modeluuid.UUID, 0, len(events))
	for _, event := range events {
		if event.ID.IsNull() {
			continue
		}
		ids = append(ids, event.ID)
	}
	return ids, nil
}

func (r *gormDroppedEvents) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("expires_at <= ?", now).
		Delete(&coremodel.DroppedEvent{})
	return result.RowsAffected, result.Error
}

type gormAllowlistSenders struct{ db *gorm.DB }

func (r *gormAllowlistSenders) Save(ctx context.Context, sender *coremodel.AllowlistSender) error {
	sender.SenderKey = clean(sender.SenderKey)
	sender.SenderLabel = strings.TrimSpace(sender.SenderLabel)
	if sender.ID.IsNull() {
		existing, err := r.GetBySourceBindingAndSenderKey(ctx, sender.SourceBindingID, sender.SenderKey)
		if err != nil {
			return err
		}
		if existing != nil {
			sender.ID = existing.ID
		}
	}
	ensureID(&sender.ID)
	return r.db.WithContext(ctx).Save(sender).Error
}

func (r *gormAllowlistSenders) GetBySourceBindingAndSenderKey(ctx context.Context, sourceBindingID modeluuid.UUID, senderKey string) (*coremodel.AllowlistSender, error) {
	var sender coremodel.AllowlistSender
	if err := first(r.db.WithContext(ctx).
		Where("source_binding_id = ? AND sender_key = ?", sourceBindingID, clean(senderKey)).
		First(&sender)); err != nil {
		return nil, err
	}
	if sender.ID.IsNull() {
		return nil, nil
	}
	return &sender, nil
}

func (r *gormAllowlistSenders) ListBySourceBindingID(ctx context.Context, sourceBindingID modeluuid.UUID) ([]coremodel.AllowlistSender, error) {
	var senders []coremodel.AllowlistSender
	err := r.db.WithContext(ctx).
		Where("source_binding_id = ?", sourceBindingID).
		Order("sender_key ASC").
		Find(&senders).Error
	return senders, err
}

func (r *gormAllowlistSenders) DeleteBySourceBindingAndSenderKey(ctx context.Context, sourceBindingID modeluuid.UUID, senderKey string) (bool, error) {
	result := r.db.WithContext(ctx).
		Where("source_binding_id = ? AND sender_key = ?", sourceBindingID, clean(senderKey)).
		Delete(&coremodel.AllowlistSender{})
	return result.RowsAffected > 0, result.Error
}

type gormThreadComponentMappings struct{ db *gorm.DB }

func (r *gormThreadComponentMappings) Save(ctx context.Context, mapping *coremodel.ThreadComponentMapping) error {
	mapping.ComponentThreadID = clean(mapping.ComponentThreadID)
	if mapping.ID.IsNull() {
		existing, err := r.GetByThreadAndComponent(ctx, mapping.ThreadID, mapping.ComponentID)
		if err != nil {
			return err
		}
		if existing != nil {
			mapping.ID = existing.ID
		}
	}
	ensureID(&mapping.ID)
	return r.db.WithContext(ctx).Save(mapping).Error
}

func (r *gormThreadComponentMappings) GetByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) (*coremodel.ThreadComponentMapping, error) {
	var mapping coremodel.ThreadComponentMapping
	if err := first(r.db.WithContext(ctx).
		Where("thread_id = ? AND component_id = ?", threadID, componentID).
		First(&mapping)); err != nil {
		return nil, err
	}
	if mapping.ID.IsNull() {
		return nil, nil
	}
	return &mapping, nil
}

func (r *gormThreadComponentMappings) FindByChatComponentAndThreadID(ctx context.Context, chatID modeluuid.UUID, componentID modeluuid.UUID, componentThreadID string) (*coremodel.ThreadComponentMapping, error) {
	var mapping coremodel.ThreadComponentMapping
	if err := first(r.db.WithContext(ctx).
		Where("chat_id = ? AND component_id = ? AND component_thread_id = ?", chatID, componentID, clean(componentThreadID)).
		First(&mapping)); err != nil {
		return nil, err
	}
	if mapping.ID.IsNull() {
		return nil, nil
	}
	return &mapping, nil
}

func (r *gormThreadComponentMappings) ListByChatID(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.ThreadComponentMapping, error) {
	var mappings []coremodel.ThreadComponentMapping
	err := r.db.WithContext(ctx).
		Where("chat_id = ?", chatID).
		Order("created_at ASC").
		Find(&mappings).Error
	return mappings, err
}

func (r *gormThreadComponentMappings) DeleteByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("thread_id = ? AND component_id = ?", threadID, componentID).
		Delete(&coremodel.ThreadComponentMapping{}).
		Error
}

func (r *gormThreadComponentMappings) DeleteByComponentID(ctx context.Context, componentID modeluuid.UUID) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("component_id = ?", componentID).
		Delete(&coremodel.ThreadComponentMapping{})
	return result.RowsAffected, result.Error
}

type gormThreadComponentStates struct{ db *gorm.DB }

func (r *gormThreadComponentStates) Save(ctx context.Context, state *coremodel.ThreadComponentState) error {
	state.StateJSON = clean(state.StateJSON)
	if state.ID.IsNull() {
		existing, err := r.GetByThreadAndComponent(ctx, state.ThreadID, state.ComponentID)
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

func (r *gormThreadComponentStates) GetByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) (*coremodel.ThreadComponentState, error) {
	var state coremodel.ThreadComponentState
	if err := first(r.db.WithContext(ctx).
		Where("thread_id = ? AND component_id = ?", threadID, componentID).
		First(&state)); err != nil {
		return nil, err
	}
	if state.ID.IsNull() {
		return nil, nil
	}
	return &state, nil
}

func (r *gormThreadComponentStates) DeleteByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("thread_id = ? AND component_id = ?", threadID, componentID).
		Delete(&coremodel.ThreadComponentState{}).
		Error
}

func (r *gormThreadComponentStates) DeleteByComponentID(ctx context.Context, componentID modeluuid.UUID) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("component_id = ?", componentID).
		Delete(&coremodel.ThreadComponentState{})
	return result.RowsAffected, result.Error
}

type gormMessages struct{ db *gorm.DB }

func (r *gormMessages) Append(ctx context.Context, message *coremodel.ThreadMessage) error {
	ensureID(&message.ID)
	return r.db.WithContext(ctx).Create(message).Error
}

func (r *gormMessages) ListByThreadID(ctx context.Context, threadID modeluuid.UUID) ([]coremodel.ThreadMessage, error) {
	var out []coremodel.ThreadMessage
	err := r.db.WithContext(ctx).Where("thread_id = ?", threadID).Order("created_at ASC").Find(&out).Error
	return out, err
}

func (r *gormMessages) ListByThreadIDPage(ctx context.Context, threadID modeluuid.UUID, afterMessageID modeluuid.UUID, limit int) ([]coremodel.ThreadMessage, string, error) {
	if limit <= 0 {
		return nil, "", fmt.Errorf("message page limit must be positive")
	}
	query := r.db.WithContext(ctx).Where("thread_id = ?", threadID)
	if !afterMessageID.IsNull() {
		var cursor coremodel.ThreadMessage
		if err := r.db.WithContext(ctx).Where("thread_id = ? and id = ?", threadID, afterMessageID).First(&cursor).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil, "", &repository.ShortIDNotFoundError{Ref: afterMessageID.String()}
			}
			return nil, "", err
		}
		query = query.Where("created_at > ? OR (created_at = ? AND id > ?)", cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
		query = query.Order("created_at ASC").Order("id ASC")
	} else {
		query = query.Order("created_at DESC").Order("id DESC")
	}
	var rows []coremodel.ThreadMessage
	if err := query.Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		if !afterMessageID.IsNull() {
			next = rows[limit-1].ID.String()
		}
		rows = rows[:limit]
	}
	if afterMessageID.IsNull() {
		for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
			rows[i], rows[j] = rows[j], rows[i]
		}
	}
	return rows, next, nil
}

func (r *gormMessages) CountByThreadIDSince(ctx context.Context, threadID modeluuid.UUID, since *time.Time) (int64, error) {
	query := r.db.WithContext(ctx).Model(&coremodel.ThreadMessage{}).Where("thread_id = ?", threadID)
	if since != nil {
		query = query.Where("created_at > ?", since.UTC())
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *gormMessages) DeleteByThreadID(ctx context.Context, threadID modeluuid.UUID) (int64, error) {
	result := r.db.WithContext(ctx).Where("thread_id = ?", threadID).Delete(&coremodel.ThreadMessage{})
	return result.RowsAffected, result.Error
}

type gormArtifacts struct {
	db          *gorm.DB
	artifactDir string
}

func (r *gormArtifacts) Append(ctx context.Context, artifact *coremodel.Artifact) error {
	if err := r.prepareArtifactForStorage(artifact); err != nil {
		return err
	}
	ensureID(&artifact.ID)
	if err := r.db.WithContext(ctx).Create(artifact).Error; err != nil {
		r.removePreparedFile(artifact)
		return err
	}
	return nil
}

func (r *gormArtifacts) ListByMessageID(ctx context.Context, messageID modeluuid.UUID) ([]coremodel.Artifact, error) {
	var out []coremodel.Artifact
	err := r.db.WithContext(ctx).Where("message_id = ?", messageID).Order("created_at ASC").Find(&out).Error
	if err != nil {
		return nil, err
	}
	for i := range out {
		if err := r.loadFileContent(&out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (r *gormArtifacts) DeleteByThreadID(ctx context.Context, threadID modeluuid.UUID) (int64, error) {
	var artifacts []coremodel.Artifact
	if err := r.db.WithContext(ctx).Where("thread_id = ?", threadID).Find(&artifacts).Error; err != nil {
		return 0, err
	}
	result := r.db.WithContext(ctx).Where("thread_id = ?", threadID).Delete(&coremodel.Artifact{})
	if result.Error != nil {
		return 0, result.Error
	}
	for i := range artifacts {
		r.removePreparedFile(&artifacts[i])
	}
	return result.RowsAffected, nil
}

func (r *gormArtifacts) prepareArtifactForStorage(artifact *coremodel.Artifact) error {
	if artifact == nil {
		return fmt.Errorf("missing artifact")
	}
	if strings.TrimSpace(r.artifactDir) == "" {
		return fmt.Errorf("artifact storage directory is not configured")
	}
	ensureID(&artifact.ID)
	artifact.Filename = strings.TrimSpace(artifact.Filename)
	artifact.ContentType = clean(artifact.ContentType)
	artifact.Syntax = clean(artifact.Syntax)
	artifact.StorageKind = "file"
	artifact.StoragePath = clean(artifact.StoragePath)
	artifact.SHA256 = clean(artifact.SHA256)
	artifact.MetadataJSON = clean(artifact.MetadataJSON)
	artifact.Size = int64(len(artifact.Content))
	sum := sha256.Sum256(artifact.Content)
	artifact.SHA256 = hex.EncodeToString(sum[:])
	if err := os.MkdirAll(r.artifactDir, 0o755); err != nil {
		return fmt.Errorf("artifact dir: %w", err)
	}
	relPath := artifact.ID.String() + ".bin"
	target, err := r.artifactFilePath(relPath)
	if err != nil {
		return err
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, artifact.Content, 0o600); err != nil {
		return fmt.Errorf("write artifact: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit artifact: %w", err)
	}
	artifact.StoragePath = relPath
	artifact.Content = nil
	return nil
}

func (r *gormArtifacts) loadFileContent(artifact *coremodel.Artifact) error {
	if artifact == nil || strings.TrimSpace(artifact.StoragePath) == "" {
		return nil
	}
	if artifact.StorageKind != "file" {
		return fmt.Errorf("unsupported artifact storage kind %q", artifact.StorageKind)
	}
	if strings.TrimSpace(r.artifactDir) == "" {
		return fmt.Errorf("artifact storage directory is not configured")
	}
	path, err := r.artifactFilePath(artifact.StoragePath)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read artifact: %w", err)
	}
	artifact.Content = content
	return nil
}

func (r *gormArtifacts) removePreparedFile(artifact *coremodel.Artifact) {
	if artifact == nil || artifact.StorageKind != "file" || artifact.StoragePath == "" || r.artifactDir == "" {
		return
	}
	path, err := r.artifactFilePath(artifact.StoragePath)
	if err != nil {
		return
	}
	_ = os.Remove(path)
}

func (r *gormArtifacts) artifactFilePath(relPath string) (string, error) {
	relPath = filepath.Clean(strings.TrimSpace(relPath))
	if relPath == "." || relPath == ".." || filepath.IsAbs(relPath) || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe artifact path: %q", relPath)
	}
	return filepath.Join(r.artifactDir, relPath), nil
}

type gormScheduledJobs struct{ db *gorm.DB }

func (r *gormScheduledJobs) Save(ctx context.Context, job *coremodel.ScheduledJob) error {
	if job == nil {
		return fmt.Errorf("missing scheduled job")
	}
	job.Name = clean(job.Name)
	job.Every = clean(job.Every)
	job.CommandJSON = clean(job.CommandJSON)
	if job.ID.IsNull() {
		var existing coremodel.ScheduledJob
		err := r.db.WithContext(ctx).Where("name = ?", job.Name).First(&existing).Error
		if err == nil {
			job.ID = existing.ID
			job.CreatedAt = existing.CreatedAt
			if job.LastRunAt == nil {
				job.LastRunAt = existing.LastRunAt
			}
			if job.LastStatus == "" {
				job.LastStatus = existing.LastStatus
			}
			if job.LastError == "" {
				job.LastError = existing.LastError
			}
		} else if err != gorm.ErrRecordNotFound {
			return err
		}
	}
	ensureID(&job.ID)
	return r.db.WithContext(ctx).Save(job).Error
}

func (r *gormScheduledJobs) List(ctx context.Context) ([]coremodel.ScheduledJob, error) {
	var jobs []coremodel.ScheduledJob
	err := r.db.WithContext(ctx).Order("name ASC").Find(&jobs).Error
	return jobs, err
}

func (r *gormScheduledJobs) ListDue(ctx context.Context, now time.Time) ([]coremodel.ScheduledJob, error) {
	var jobs []coremodel.ScheduledJob
	err := r.db.WithContext(ctx).
		Where("enabled = ? AND next_run_at <= ?", true, now.UTC()).
		Order("next_run_at ASC").
		Order("name ASC").
		Find(&jobs).Error
	return jobs, err
}

func (r *gormScheduledJobs) DeleteByName(ctx context.Context, name string) (bool, error) {
	result := r.db.WithContext(ctx).
		Where("name = ?", clean(name)).
		Delete(&coremodel.ScheduledJob{})
	return result.RowsAffected > 0, result.Error
}

type gormTimedIntents struct{ db *gorm.DB }

func (r *gormTimedIntents) Save(ctx context.Context, intent *coremodel.TimedIntent) error {
	if intent == nil {
		return fmt.Errorf("missing timed intent")
	}
	cleanTimedIntent(intent)
	ensureID(&intent.ID)
	return r.db.WithContext(ctx).Save(intent).Error
}

func (r *gormTimedIntents) UpsertByTargetKindKey(ctx context.Context, intent *coremodel.TimedIntent) error {
	if intent == nil {
		return fmt.Errorf("missing timed intent")
	}
	cleanTimedIntent(intent)
	var existing coremodel.TimedIntent
	err := r.db.WithContext(ctx).
		Where("target_thread_id = ? AND kind = ? AND key = ?", intent.TargetThreadID, intent.Kind, intent.Key).
		First(&existing).Error
	if err == nil {
		intent.ID = existing.ID
		intent.CreatedAt = existing.CreatedAt
	} else if err != gorm.ErrRecordNotFound {
		return err
	}
	ensureID(&intent.ID)
	return r.db.WithContext(ctx).Save(intent).Error
}

func (r *gormTimedIntents) GetByTargetKindKey(ctx context.Context, targetThreadID modeluuid.UUID, kind string, key string) (*coremodel.TimedIntent, error) {
	var intent coremodel.TimedIntent
	err := r.db.WithContext(ctx).
		Where("target_thread_id = ? AND kind = ? AND key = ?", targetThreadID, clean(kind), clean(key)).
		First(&intent).Error
	if err == gorm.ErrRecordNotFound {
		return nil, &repository.ShortIDNotFoundError{Ref: clean(kind) + ":" + clean(key)}
	}
	if err != nil {
		return nil, err
	}
	return &intent, nil
}

func (r *gormTimedIntents) ListByTarget(ctx context.Context, targetThreadID modeluuid.UUID) ([]coremodel.TimedIntent, error) {
	var intents []coremodel.TimedIntent
	err := r.db.WithContext(ctx).
		Where("target_thread_id = ?", targetThreadID).
		Order("next_due_at ASC").
		Order("kind ASC").
		Order("key ASC").
		Find(&intents).Error
	return intents, err
}

func (r *gormTimedIntents) ListDue(ctx context.Context, now time.Time, limit int) ([]coremodel.TimedIntent, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("invalid timed intent limit: %d", limit)
	}
	var intents []coremodel.TimedIntent
	err := r.db.WithContext(ctx).
		Where("enabled = ? AND next_due_at IS NOT NULL AND next_due_at <= ?", true, now.UTC()).
		Order("next_due_at ASC").
		Order("target_thread_id ASC").
		Order("kind ASC").
		Order("key ASC").
		Limit(limit).
		Find(&intents).Error
	return intents, err
}

func (r *gormTimedIntents) NextDue(ctx context.Context) (*time.Time, error) {
	var intent coremodel.TimedIntent
	err := r.db.WithContext(ctx).
		Where("enabled = ? AND next_due_at IS NOT NULL", true).
		Order("next_due_at ASC").
		First(&intent).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if intent.NextDueAt == nil {
		return nil, nil
	}
	next := intent.NextDueAt.UTC()
	return &next, nil
}

func (r *gormTimedIntents) DeleteByTargetKindKey(ctx context.Context, targetThreadID modeluuid.UUID, kind string, key string) (bool, error) {
	result := r.db.WithContext(ctx).
		Where("target_thread_id = ? AND kind = ? AND key = ?", targetThreadID, clean(kind), clean(key)).
		Delete(&coremodel.TimedIntent{})
	return result.RowsAffected > 0, result.Error
}

func cleanTimedIntent(intent *coremodel.TimedIntent) {
	intent.Kind = clean(intent.Kind)
	intent.Key = clean(intent.Key)
	intent.OwnerActorID = clean(intent.OwnerActorID)
	intent.Every = clean(intent.Every)
	intent.Cron = clean(intent.Cron)
	intent.Timezone = clean(intent.Timezone)
	intent.Delivery = clean(intent.Delivery)
	intent.HandlerRef = clean(intent.HandlerRef)
	intent.ParamsJSON = clean(intent.ParamsJSON)
	intent.Label = clean(intent.Label)
	intent.LastStatus = clean(intent.LastStatus)
	intent.LastError = clean(intent.LastError)
}

type gormTrustedControllers struct{ db *gorm.DB }

func (r *gormTrustedControllers) Save(ctx context.Context, controller *coremodel.TrustedController) error {
	if controller == nil {
		return fmt.Errorf("missing trusted controller")
	}
	controller.ControllerID = clean(controller.ControllerID)
	controller.DisplayName = clean(controller.DisplayName)
	controller.Fingerprint = clean(controller.Fingerprint)
	controller.CertificatePEM = clean(controller.CertificatePEM)
	if controller.ID.IsNull() {
		var existing coremodel.TrustedController
		err := r.db.WithContext(ctx).Where("fingerprint = ? OR (controller_id <> '' AND controller_id = ?)", controller.Fingerprint, controller.ControllerID).First(&existing).Error
		if err == nil {
			controller.ID = existing.ID
			controller.CreatedAt = existing.CreatedAt
		} else if err != gorm.ErrRecordNotFound {
			return err
		}
	}
	ensureID(&controller.ID)
	return r.db.WithContext(ctx).Save(controller).Error
}

func (r *gormTrustedControllers) GetByFingerprint(ctx context.Context, fingerprint string) (*coremodel.TrustedController, error) {
	var controller coremodel.TrustedController
	err := r.db.WithContext(ctx).Where("fingerprint = ? AND revoked_at IS NULL", clean(fingerprint)).First(&controller).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &controller, nil
}

func (r *gormTrustedControllers) List(ctx context.Context) ([]coremodel.TrustedController, error) {
	var controllers []coremodel.TrustedController
	err := r.db.WithContext(ctx).Order("display_name ASC").Find(&controllers).Error
	return controllers, err
}

func (r *gormTrustedControllers) RevokeByFingerprint(ctx context.Context, fingerprint string) (bool, error) {
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&coremodel.TrustedController{}).Where("fingerprint = ?", clean(fingerprint)).Update("revoked_at", &now)
	return result.RowsAffected > 0, result.Error
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
