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
	}
}

func (s *GORMStorage) AutoMigrate(ctx context.Context) error {
	if err := s.migrateProviderChannelColumns(ctx); err != nil {
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
	); err != nil {
		return err
	}
	if err := s.migrateThreadMessageRoles(ctx); err != nil {
		return err
	}
	return nil
}

func (s *GORMStorage) migrateProviderChannelColumns(ctx context.Context) error {
	migrator := s.db.WithContext(ctx).Migrator()
	for _, migration := range []struct {
		model any
		old   string
		new   string
	}{
		{model: &coremodel.ChatComponent{}, old: "external_chat_id", new: "external_channel_id"},
		{model: &coremodel.InboundDrop{}, old: "external_chat_id", new: "external_channel_id"},
	} {
		if !migrator.HasTable(migration.model) {
			continue
		}
		if migrator.HasColumn(migration.model, migration.old) && !migrator.HasColumn(migration.model, migration.new) {
			if err := migrator.RenameColumn(migration.model, migration.old, migration.new); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *GORMStorage) migrateThreadMessageRoles(ctx context.Context) error {
	return s.db.WithContext(ctx).Exec(`
		UPDATE thread_messages
		SET role = CASE
			WHEN kind = ? THEN ?
			WHEN kind = ? THEN ?
			WHEN kind = ? THEN ?
			WHEN kind = ? THEN ?
			WHEN direction = ? THEN ?
			WHEN direction = ? THEN ?
			ELSE role
		END
		WHERE role IS NULL OR role = ''
	`,
		coremodel.MessageKindUser, coremodel.MessageRoleUser,
		coremodel.MessageKindAgent, coremodel.MessageRoleAgent,
		coremodel.MessageKindSystem, coremodel.MessageRoleSystem,
		coremodel.MessageKindEvent, coremodel.MessageRoleSystem,
		coremodel.MessageDirectionInbound, coremodel.MessageRoleUser,
		coremodel.MessageDirectionOutbound, coremodel.MessageRoleAgent,
	).Error
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
	component.HomePath = clean(component.HomePath)
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
