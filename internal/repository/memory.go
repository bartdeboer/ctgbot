package repository

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type MemoryStorage struct {
	mu sync.Mutex

	chats               map[modeluuid.UUID]coremodel.Chat
	threads             map[modeluuid.UUID]coremodel.Thread
	components          map[modeluuid.UUID]coremodel.Component
	chatComponents      map[modeluuid.UUID]coremodel.ChatComponent
	threadComponentMaps map[modeluuid.UUID]coremodel.ThreadComponentMapping
	threadComponentRows map[modeluuid.UUID]coremodel.ThreadComponentState
	messages            map[modeluuid.UUID]coremodel.ThreadMessage
	artifacts           map[modeluuid.UUID]coremodel.Artifact
}

func NewMemory() *MemoryStorage {
	return &MemoryStorage{
		chats:               map[modeluuid.UUID]coremodel.Chat{},
		threads:             map[modeluuid.UUID]coremodel.Thread{},
		components:          map[modeluuid.UUID]coremodel.Component{},
		chatComponents:      map[modeluuid.UUID]coremodel.ChatComponent{},
		threadComponentMaps: map[modeluuid.UUID]coremodel.ThreadComponentMapping{},
		threadComponentRows: map[modeluuid.UUID]coremodel.ThreadComponentState{},
		messages:            map[modeluuid.UUID]coremodel.ThreadMessage{},
		artifacts:           map[modeluuid.UUID]coremodel.Artifact{},
	}
}

func (s *MemoryStorage) Transaction(ctx context.Context, fn func(Storage) error) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	tx := s.cloneLocked()
	s.mu.Unlock()

	if err := fn(tx); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.replaceLocked(tx)
	return nil
}

func (s *MemoryStorage) Chats() ChatRepository                   { return memoryChats{s} }
func (s *MemoryStorage) Threads() ThreadRepository               { return memoryThreads{s} }
func (s *MemoryStorage) Components() ComponentRepository         { return memoryComponents{s} }
func (s *MemoryStorage) ChatComponents() ChatComponentRepository { return memoryChatComponents{s} }
func (s *MemoryStorage) ThreadComponentMappings() ThreadComponentMappingRepository {
	return memoryThreadMappings{s}
}
func (s *MemoryStorage) ThreadComponentStates() ThreadComponentStateRepository {
	return memoryThreadStates{s}
}
func (s *MemoryStorage) Messages() MessageRepository   { return memoryMessages{s} }
func (s *MemoryStorage) Artifacts() ArtifactRepository { return memoryArtifacts{s} }

func (s *MemoryStorage) cloneLocked() *MemoryStorage {
	clone := NewMemory()
	for k, v := range s.chats {
		clone.chats[k] = v
	}
	for k, v := range s.threads {
		clone.threads[k] = v
	}
	for k, v := range s.components {
		clone.components[k] = v
	}
	for k, v := range s.chatComponents {
		clone.chatComponents[k] = v
	}
	for k, v := range s.threadComponentMaps {
		clone.threadComponentMaps[k] = v
	}
	for k, v := range s.threadComponentRows {
		clone.threadComponentRows[k] = v
	}
	for k, v := range s.messages {
		clone.messages[k] = v
	}
	for k, v := range s.artifacts {
		clone.artifacts[k] = v
	}
	return clone
}

func (s *MemoryStorage) replaceLocked(next *MemoryStorage) {
	if s == nil || next == nil {
		return
	}
	s.chats = next.chats
	s.threads = next.threads
	s.components = next.components
	s.chatComponents = next.chatComponents
	s.threadComponentMaps = next.threadComponentMaps
	s.threadComponentRows = next.threadComponentRows
	s.messages = next.messages
	s.artifacts = next.artifacts
}

type memoryChats struct{ s *MemoryStorage }

func (r memoryChats) Save(ctx context.Context, chat *coremodel.Chat) error {
	_ = ctx
	if chat == nil {
		return nil
	}
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	now := time.Now()
	if chat.ID.IsNull() {
		chat.ID = modeluuid.New()
		chat.CreatedAt = now
	} else if existing, ok := r.s.chats[chat.ID]; ok && chat.CreatedAt.IsZero() {
		chat.CreatedAt = existing.CreatedAt
	}
	chat.UpdatedAt = now
	r.s.chats[chat.ID] = *chat
	return nil
}

func (r memoryChats) GetByID(ctx context.Context, chatID modeluuid.UUID) (*coremodel.Chat, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	chat, ok := r.s.chats[chatID]
	if !ok {
		return nil, nil
	}
	copy := chat
	return &copy, nil
}

func (r memoryChats) List(ctx context.Context) ([]coremodel.Chat, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []coremodel.Chat
	for _, chat := range r.s.chats {
		out = append(out, chat)
	}
	return out, nil
}

type memoryThreads struct{ s *MemoryStorage }

func (r memoryThreads) Save(ctx context.Context, thread *coremodel.Thread) error {
	_ = ctx
	if thread == nil {
		return nil
	}
	thread.Label = strings.TrimSpace(thread.Label)
	thread.CodexModel = strings.TrimSpace(thread.CodexModel)
	thread.CodexReasoningEffort = strings.TrimSpace(thread.CodexReasoningEffort)
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	now := time.Now()
	if thread.ID.IsNull() {
		thread.ID = modeluuid.New()
		thread.CreatedAt = now
	} else if existing, ok := r.s.threads[thread.ID]; ok && thread.CreatedAt.IsZero() {
		thread.CreatedAt = existing.CreatedAt
	}
	thread.UpdatedAt = now
	r.s.threads[thread.ID] = *thread
	return nil
}

func (r memoryThreads) GetByID(ctx context.Context, threadID modeluuid.UUID) (*coremodel.Thread, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	thread, ok := r.s.threads[threadID]
	if !ok {
		return nil, nil
	}
	copy := thread
	return &copy, nil
}

func (r memoryThreads) ListByChatID(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.Thread, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []coremodel.Thread
	for _, thread := range r.s.threads {
		if thread.ChatID == chatID {
			out = append(out, thread)
		}
	}
	return out, nil
}

type memoryComponents struct{ s *MemoryStorage }

func (r memoryComponents) Save(ctx context.Context, component *coremodel.Component) error {
	_ = ctx
	if component == nil {
		return nil
	}
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	now := time.Now()
	if component.ID.IsNull() {
		component.ID = modeluuid.New()
		component.CreatedAt = now
	} else if existing, ok := r.s.components[component.ID]; ok && component.CreatedAt.IsZero() {
		component.CreatedAt = existing.CreatedAt
	}
	component.UpdatedAt = now
	r.s.components[component.ID] = *component
	return nil
}

func (r memoryComponents) GetByID(ctx context.Context, componentID modeluuid.UUID) (*coremodel.Component, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	component, ok := r.s.components[componentID]
	if !ok {
		return nil, nil
	}
	copy := component
	return &copy, nil
}

func (r memoryComponents) GetDefaultByType(ctx context.Context, componentType string) (*coremodel.Component, error) {
	_ = ctx
	componentType = strings.TrimSpace(componentType)
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, component := range r.s.components {
		if component.Enabled && component.Type == componentType && component.IsDefault {
			copy := component
			return &copy, nil
		}
	}
	return nil, nil
}

func (r memoryComponents) GetByTypeAndName(ctx context.Context, componentType string, name string) (*coremodel.Component, error) {
	_ = ctx
	componentType = strings.TrimSpace(componentType)
	name = strings.TrimSpace(name)
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, component := range r.s.components {
		if component.Type == componentType && component.Name == name {
			copy := component
			return &copy, nil
		}
	}
	return nil, nil
}

func (r memoryComponents) ListEnabled(ctx context.Context) ([]coremodel.Component, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []coremodel.Component
	for _, component := range r.s.components {
		if component.Enabled {
			out = append(out, component)
		}
	}
	return out, nil
}

type memoryChatComponents struct{ s *MemoryStorage }

func (r memoryChatComponents) Save(ctx context.Context, binding *coremodel.ChatComponent) error {
	_ = ctx
	if binding == nil {
		return nil
	}
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	now := time.Now()
	if binding.ID.IsNull() {
		binding.ID = modeluuid.New()
		binding.CreatedAt = now
	} else if existing, ok := r.s.chatComponents[binding.ID]; ok && binding.CreatedAt.IsZero() {
		binding.CreatedAt = existing.CreatedAt
	}
	binding.UpdatedAt = now
	r.s.chatComponents[binding.ID] = *binding
	return nil
}

func (r memoryChatComponents) GetByChatComponentRole(ctx context.Context, chatID modeluuid.UUID, componentID modeluuid.UUID, role coremodel.ChatComponentRole) (*coremodel.ChatComponent, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, binding := range r.s.chatComponents {
		if binding.ChatID == chatID && binding.ComponentID == componentID && binding.Role == role {
			copy := binding
			return &copy, nil
		}
	}
	return nil, nil
}

func (r memoryChatComponents) ListEnabledByChatID(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.ChatComponent, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []coremodel.ChatComponent
	for _, binding := range r.s.chatComponents {
		if binding.ChatID == chatID && binding.Enabled {
			out = append(out, binding)
		}
	}
	return out, nil
}

func (r memoryChatComponents) FindByComponentRoleAndExternalChatID(ctx context.Context, componentID modeluuid.UUID, role coremodel.ChatComponentRole, externalChatID string) (*coremodel.ChatComponent, error) {
	_ = ctx
	externalChatID = strings.TrimSpace(externalChatID)
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, binding := range r.s.chatComponents {
		if binding.ComponentID == componentID && binding.Role == role && strings.TrimSpace(binding.ExternalChatID) == externalChatID && binding.Enabled {
			copy := binding
			return &copy, nil
		}
	}
	return nil, nil
}

type memoryThreadMappings struct{ s *MemoryStorage }

func (r memoryThreadMappings) Save(ctx context.Context, mapping *coremodel.ThreadComponentMapping) error {
	_ = ctx
	if mapping == nil {
		return nil
	}
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	now := time.Now()
	if mapping.ID.IsNull() {
		mapping.ID = modeluuid.New()
		mapping.CreatedAt = now
	} else if existing, ok := r.s.threadComponentMaps[mapping.ID]; ok && mapping.CreatedAt.IsZero() {
		mapping.CreatedAt = existing.CreatedAt
	}
	mapping.UpdatedAt = now
	r.s.threadComponentMaps[mapping.ID] = *mapping
	return nil
}

func (r memoryThreadMappings) GetByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) (*coremodel.ThreadComponentMapping, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, mapping := range r.s.threadComponentMaps {
		if mapping.ThreadID == threadID && mapping.ComponentID == componentID {
			copy := mapping
			return &copy, nil
		}
	}
	return nil, nil
}

func (r memoryThreadMappings) FindByChatComponentAndThreadID(ctx context.Context, chatID modeluuid.UUID, componentID modeluuid.UUID, componentThreadID string) (*coremodel.ThreadComponentMapping, error) {
	_ = ctx
	componentThreadID = strings.TrimSpace(componentThreadID)
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, mapping := range r.s.threadComponentMaps {
		if mapping.ChatID == chatID && mapping.ComponentID == componentID && strings.TrimSpace(mapping.ComponentThreadID) == componentThreadID {
			copy := mapping
			return &copy, nil
		}
	}
	return nil, nil
}

func (r memoryThreadMappings) DeleteByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) error {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for id, mapping := range r.s.threadComponentMaps {
		if mapping.ThreadID == threadID && mapping.ComponentID == componentID {
			delete(r.s.threadComponentMaps, id)
		}
	}
	return nil
}

type memoryThreadStates struct{ s *MemoryStorage }

func (r memoryThreadStates) Save(ctx context.Context, state *coremodel.ThreadComponentState) error {
	_ = ctx
	if state == nil {
		return nil
	}
	state.StateJSON = strings.TrimSpace(state.StateJSON)
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	now := time.Now()
	if state.ID.IsNull() {
		for id, existing := range r.s.threadComponentRows {
			if existing.ThreadID == state.ThreadID && existing.ComponentID == state.ComponentID {
				state.ID = id
				state.CreatedAt = existing.CreatedAt
				break
			}
		}
	}
	if state.ID.IsNull() {
		state.ID = modeluuid.New()
		state.CreatedAt = now
	} else if existing, ok := r.s.threadComponentRows[state.ID]; ok && state.CreatedAt.IsZero() {
		state.CreatedAt = existing.CreatedAt
	}
	state.UpdatedAt = now
	r.s.threadComponentRows[state.ID] = *state
	return nil
}

func (r memoryThreadStates) GetByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) (*coremodel.ThreadComponentState, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, state := range r.s.threadComponentRows {
		if state.ThreadID == threadID && state.ComponentID == componentID {
			copy := state
			return &copy, nil
		}
	}
	return nil, nil
}

func (r memoryThreadStates) DeleteByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) error {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for id, state := range r.s.threadComponentRows {
		if state.ThreadID == threadID && state.ComponentID == componentID {
			delete(r.s.threadComponentRows, id)
		}
	}
	return nil
}

type memoryMessages struct{ s *MemoryStorage }

func (r memoryMessages) Append(ctx context.Context, message *coremodel.ThreadMessage) error {
	_ = ctx
	if message == nil {
		return nil
	}
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	now := time.Now()
	if message.ID.IsNull() {
		message.ID = modeluuid.New()
		message.CreatedAt = now
	}
	message.UpdatedAt = now
	r.s.messages[message.ID] = *message
	return nil
}

func (r memoryMessages) ListByThreadID(ctx context.Context, threadID modeluuid.UUID) ([]coremodel.ThreadMessage, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []coremodel.ThreadMessage
	for _, message := range r.s.messages {
		if message.ThreadID == threadID {
			out = append(out, message)
		}
	}
	return out, nil
}

type memoryArtifacts struct{ s *MemoryStorage }

func (r memoryArtifacts) Append(ctx context.Context, artifact *coremodel.Artifact) error {
	_ = ctx
	if artifact == nil {
		return nil
	}
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	now := time.Now()
	if artifact.ID.IsNull() {
		artifact.ID = modeluuid.New()
		artifact.CreatedAt = now
	}
	artifact.UpdatedAt = now
	artifact.Content = append([]byte(nil), artifact.Content...)
	r.s.artifacts[artifact.ID] = *artifact
	return nil
}

func (r memoryArtifacts) ListByMessageID(ctx context.Context, messageID modeluuid.UUID) ([]coremodel.Artifact, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []coremodel.Artifact
	for _, artifact := range r.s.artifacts {
		if artifact.MessageID == messageID {
			copy := artifact
			copy.Content = append([]byte(nil), artifact.Content...)
			out = append(out, copy)
		}
	}
	return out, nil
}
