package gormstorage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	"gorm.io/gorm"
)

type GORMStorage struct {
	db             *gorm.DB
	chats          *gormChats
	threads        *gormThreads
	components     *gormComponents
	chatComponents *gormChatComponents
	threadMappings *gormThreadComponentMappings
	threadStates   *gormThreadComponentStates
	messages       *gormMessages
	artifacts      *gormArtifacts
}

func New(db *gorm.DB) *GORMStorage {
	return NewWithArtifactDir(db, "")
}

func NewWithArtifactDir(db *gorm.DB, artifactDir string) *GORMStorage {
	return &GORMStorage{
		db:             db,
		chats:          &gormChats{db: db},
		threads:        &gormThreads{db: db},
		components:     &gormComponents{db: db},
		chatComponents: &gormChatComponents{db: db},
		threadMappings: &gormThreadComponentMappings{db: db},
		threadStates:   &gormThreadComponentStates{db: db},
		messages:       &gormMessages{db: db},
		artifacts:      &gormArtifacts{db: db, artifactDir: clean(artifactDir)},
	}
}

func (s *GORMStorage) AutoMigrate(ctx context.Context) error {
	if err := s.db.WithContext(ctx).AutoMigrate(
		&coremodel.Chat{},
		&coremodel.Thread{},
		&coremodel.Component{},
		&coremodel.ChatComponent{},
		&coremodel.ThreadComponentMapping{},
		&coremodel.ThreadComponentState{},
		&coremodel.ThreadMessage{},
		&coremodel.Artifact{},
	); err != nil {
		return err
	}
	if s == nil || s.db == nil {
		return nil
	}
	tx := s.db.WithContext(ctx)
	if tx.Migrator().HasTable("artifacts") && tx.Migrator().HasColumn("artifacts", "content") {
		if err := tx.Migrator().DropColumn("artifacts", "content"); err != nil {
			return fmt.Errorf("drop artifacts.content: %w", err)
		}
	}
	return nil
}

func (s *GORMStorage) Transaction(ctx context.Context, fn func(repository.Storage) error) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("missing db")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(New(tx))
	})
}

func (s *GORMStorage) Chats() repository.ChatRepository                   { return s.chats }
func (s *GORMStorage) Threads() repository.ThreadRepository               { return s.threads }
func (s *GORMStorage) Components() repository.ComponentRepository         { return s.components }
func (s *GORMStorage) ChatComponents() repository.ChatComponentRepository { return s.chatComponents }
func (s *GORMStorage) ThreadComponentMappings() repository.ThreadComponentMappingRepository {
	return s.threadMappings
}
func (s *GORMStorage) ThreadComponentStates() repository.ThreadComponentStateRepository {
	return s.threadStates
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

type gormThreads struct{ db *gorm.DB }

func (r *gormThreads) Save(ctx context.Context, thread *coremodel.Thread) error {
	thread.Label = strings.TrimSpace(thread.Label)
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

type gormChatComponents struct{ db *gorm.DB }

func (r *gormChatComponents) Save(ctx context.Context, binding *coremodel.ChatComponent) error {
	binding.ExternalChatID = clean(binding.ExternalChatID)
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

func (r *gormChatComponents) FindByComponentRoleAndExternalChatID(ctx context.Context, componentID modeluuid.UUID, role coremodel.ChatComponentRole, externalChatID string) (*coremodel.ChatComponent, error) {
	var binding coremodel.ChatComponent
	if err := first(r.db.WithContext(ctx).
		Where("component_id = ? AND role = ? AND external_chat_id = ? AND enabled = ?", componentID, role, clean(externalChatID), true).
		First(&binding)); err != nil {
		return nil, err
	}
	if binding.ID.IsNull() {
		return nil, nil
	}
	return &binding, nil
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

func (r *gormThreadComponentMappings) DeleteByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("thread_id = ? AND component_id = ?", threadID, componentID).
		Delete(&coremodel.ThreadComponentMapping{}).
		Error
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

func requireDB(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("missing db")
	}
	return nil
}
