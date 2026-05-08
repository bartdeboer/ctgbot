package gormstorage

import (
	"context"
	"encoding/json"
	"fmt"
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
	return &GORMStorage{
		db:             db,
		chats:          &gormChats{db: db},
		threads:        &gormThreads{db: db},
		components:     &gormComponents{db: db},
		chatComponents: &gormChatComponents{db: db},
		threadMappings: &gormThreadComponentMappings{db: db},
		threadStates:   &gormThreadComponentStates{db: db},
		messages:       &gormMessages{db: db},
		artifacts:      &gormArtifacts{db: db},
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
	return s.migrateLegacyCodexThreadState(ctx)
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

type legacyCodexThreadRow struct {
	ThreadID             modeluuid.UUID `gorm:"column:thread_id"`
	ChatID               modeluuid.UUID `gorm:"column:chat_id"`
	KeepRunning          bool           `gorm:"column:keep_running"`
	CodexModel           string         `gorm:"column:codex_model"`
	CodexReasoningEffort string         `gorm:"column:codex_reasoning_effort"`
}

type legacyCodexBindingRow struct {
	ComponentID modeluuid.UUID `gorm:"column:component_id"`
}

type legacyCodexThreadState struct {
	KeepRunning     *bool  `json:"keep_running,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

func (s *GORMStorage) migrateLegacyCodexThreadState(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	tx := s.db.WithContext(ctx)
	if !tx.Migrator().HasTable("threads") {
		return nil
	}
	hasKeepRunning := tx.Migrator().HasColumn("threads", "keep_running")
	hasModel := tx.Migrator().HasColumn("threads", "codex_model")
	hasEffort := tx.Migrator().HasColumn("threads", "codex_reasoning_effort")
	if !hasKeepRunning && !hasModel && !hasEffort {
		return nil
	}
	selectCols := []string{"id AS thread_id", "chat_id"}
	whereClauses := make([]string, 0, 3)
	if hasKeepRunning {
		selectCols = append(selectCols, "keep_running")
		whereClauses = append(whereClauses, "keep_running = TRUE")
	} else {
		selectCols = append(selectCols, "FALSE AS keep_running")
	}
	if hasModel {
		selectCols = append(selectCols, "codex_model")
		whereClauses = append(whereClauses, "TRIM(IFNULL(codex_model, '')) <> ''")
	} else {
		selectCols = append(selectCols, "'' AS codex_model")
	}
	if hasEffort {
		selectCols = append(selectCols, "codex_reasoning_effort")
		whereClauses = append(whereClauses, "TRIM(IFNULL(codex_reasoning_effort, '')) <> ''")
	} else {
		selectCols = append(selectCols, "'' AS codex_reasoning_effort")
	}

	var legacyRows []legacyCodexThreadRow
	legacyRowsQuery := fmt.Sprintf(
		"SELECT %s FROM threads WHERE %s",
		strings.Join(selectCols, ", "),
		strings.Join(whereClauses, " OR "),
	)
	if err := tx.Raw(legacyRowsQuery).Scan(&legacyRows).Error; err != nil {
		return err
	}
	if len(legacyRows) == 0 {
		return nil
	}

	return tx.Transaction(func(tx *gorm.DB) error {
		storage := New(tx)
		for _, legacy := range legacyRows {
			componentIDs, err := codexComponentIDsByChat(tx, legacy.ChatID)
			if err != nil {
				return err
			}
			if len(componentIDs) == 0 {
				continue
			}
			updateAssignments := make([]string, 0, 3)
			updateArgs := make([]any, 0, 4)
			if hasKeepRunning {
				updateAssignments = append(updateAssignments, "keep_running = ?")
				updateArgs = append(updateArgs, false)
			}
			if hasModel {
				updateAssignments = append(updateAssignments, "codex_model = ?")
				updateArgs = append(updateArgs, "")
			}
			if hasEffort {
				updateAssignments = append(updateAssignments, "codex_reasoning_effort = ?")
				updateArgs = append(updateArgs, "")
			}
			updateArgs = append(updateArgs, legacy.ThreadID)
			for _, componentID := range componentIDs {
				row, err := storage.ThreadComponentStates().GetByThreadAndComponent(ctx, legacy.ThreadID, componentID)
				if err != nil {
					return err
				}
				state, err := decodeLegacyCodexThreadState(row)
				if err != nil {
					return err
				}
				if legacy.KeepRunning && state.KeepRunning == nil {
					state.KeepRunning = boolPtr(true)
				}
				if state.Model == "" {
					state.Model = clean(legacy.CodexModel)
				}
				if state.ReasoningEffort == "" {
					state.ReasoningEffort = clean(legacy.CodexReasoningEffort)
				}
				if state.isZero() {
					continue
				}
				data, err := json.Marshal(state.clean())
				if err != nil {
					return err
				}
				if row == nil {
					row = &coremodel.ThreadComponentState{
						ThreadID:    legacy.ThreadID,
						ComponentID: componentID,
					}
				}
				row.ThreadID = legacy.ThreadID
				row.ComponentID = componentID
				row.StateJSON = string(data)
				if err := storage.ThreadComponentStates().Save(ctx, row); err != nil {
					return err
				}
			}
			if err := tx.Exec(
				"UPDATE threads SET "+strings.Join(updateAssignments, ", ")+" WHERE id = ?",
				updateArgs...,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func codexComponentIDsByChat(tx *gorm.DB, chatID modeluuid.UUID) ([]modeluuid.UUID, error) {
	var rows []legacyCodexBindingRow
	err := tx.
		Table("chat_components").
		Select("DISTINCT chat_components.component_id AS component_id").
		Joins("JOIN components ON components.id = chat_components.component_id").
		Where("chat_components.chat_id = ? AND chat_components.enabled = ? AND components.enabled = ? AND components.type = ?", chatID, true, true, "codex").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]modeluuid.UUID, 0, len(rows))
	for _, row := range rows {
		if row.ComponentID.IsNull() {
			continue
		}
		out = append(out, row.ComponentID)
	}
	return out, nil
}

func decodeLegacyCodexThreadState(row *coremodel.ThreadComponentState) (legacyCodexThreadState, error) {
	if row == nil || strings.TrimSpace(row.StateJSON) == "" {
		return legacyCodexThreadState{}, nil
	}
	var state legacyCodexThreadState
	if err := json.Unmarshal([]byte(row.StateJSON), &state); err != nil {
		return legacyCodexThreadState{}, fmt.Errorf("decode codex thread state thread=%s component=%s: %w", row.ThreadID, row.ComponentID, err)
	}
	return state.clean(), nil
}

func (s legacyCodexThreadState) clean() legacyCodexThreadState {
	s.Model = clean(s.Model)
	s.ReasoningEffort = clean(s.ReasoningEffort)
	if s.KeepRunning != nil && !*s.KeepRunning {
		s.KeepRunning = nil
	}
	return s
}

func (s legacyCodexThreadState) isZero() bool {
	s = s.clean()
	return s.KeepRunning == nil && s.Model == "" && s.ReasoningEffort == ""
}

func boolPtr(value bool) *bool {
	v := value
	return &v
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

type gormArtifacts struct{ db *gorm.DB }

func (r *gormArtifacts) Append(ctx context.Context, artifact *coremodel.Artifact) error {
	ensureID(&artifact.ID)
	return r.db.WithContext(ctx).Create(artifact).Error
}

func (r *gormArtifacts) ListByMessageID(ctx context.Context, messageID modeluuid.UUID) ([]coremodel.Artifact, error) {
	var out []coremodel.Artifact
	err := r.db.WithContext(ctx).Where("message_id = ?", messageID).Order("created_at ASC").Find(&out).Error
	return out, err
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
