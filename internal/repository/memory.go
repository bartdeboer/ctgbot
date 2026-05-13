package repository

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type MemoryStorage struct {
	mu sync.Mutex

	chats                   map[modeluuid.UUID]coremodel.Chat
	threads                 map[modeluuid.UUID]coremodel.Thread
	components              map[modeluuid.UUID]coremodel.Component
	chatComponents          map[modeluuid.UUID]coremodel.ChatComponent
	inboundFilterBindings   map[modeluuid.UUID]coremodel.InboundFilterBinding
	inboundDrops            map[modeluuid.UUID]coremodel.InboundDrop
	droppedEvents           map[modeluuid.UUID]coremodel.DroppedEvent
	allowlistSenders        map[modeluuid.UUID]coremodel.AllowlistSender
	threadComponentMappings map[modeluuid.UUID]coremodel.ThreadComponentMapping
	threadComponentStates   map[modeluuid.UUID]coremodel.ThreadComponentState
	messages                map[modeluuid.UUID]coremodel.ThreadMessage
	artifacts               map[modeluuid.UUID]coremodel.Artifact
}

func NewMemory() *MemoryStorage {
	return &MemoryStorage{
		chats:                   map[modeluuid.UUID]coremodel.Chat{},
		threads:                 map[modeluuid.UUID]coremodel.Thread{},
		components:              map[modeluuid.UUID]coremodel.Component{},
		chatComponents:          map[modeluuid.UUID]coremodel.ChatComponent{},
		inboundFilterBindings:   map[modeluuid.UUID]coremodel.InboundFilterBinding{},
		inboundDrops:            map[modeluuid.UUID]coremodel.InboundDrop{},
		droppedEvents:           map[modeluuid.UUID]coremodel.DroppedEvent{},
		allowlistSenders:        map[modeluuid.UUID]coremodel.AllowlistSender{},
		threadComponentMappings: map[modeluuid.UUID]coremodel.ThreadComponentMapping{},
		threadComponentStates:   map[modeluuid.UUID]coremodel.ThreadComponentState{},
		messages:                map[modeluuid.UUID]coremodel.ThreadMessage{},
		artifacts:               map[modeluuid.UUID]coremodel.Artifact{},
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
func (s *MemoryStorage) InboundFilterBindings() InboundFilterBindingRepository {
	return memoryInboundFilterBindings{s}
}
func (s *MemoryStorage) InboundDrops() InboundDropRepository   { return memoryInboundDrops{s} }
func (s *MemoryStorage) DroppedEvents() DroppedEventRepository { return memoryDroppedEvents{s} }
func (s *MemoryStorage) AllowlistSenders() AllowlistSenderRepository {
	return memoryAllowlistSenders{s}
}
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
	for k, v := range s.inboundFilterBindings {
		clone.inboundFilterBindings[k] = v
	}
	for k, v := range s.inboundDrops {
		clone.inboundDrops[k] = v
	}
	for k, v := range s.droppedEvents {
		clone.droppedEvents[k] = v
	}
	for k, v := range s.allowlistSenders {
		clone.allowlistSenders[k] = v
	}
	for k, v := range s.threadComponentMappings {
		clone.threadComponentMappings[k] = v
	}
	for k, v := range s.threadComponentStates {
		clone.threadComponentStates[k] = v
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
	s.inboundFilterBindings = next.inboundFilterBindings
	s.inboundDrops = next.inboundDrops
	s.droppedEvents = next.droppedEvents
	s.allowlistSenders = next.allowlistSenders
	s.threadComponentMappings = next.threadComponentMappings
	s.threadComponentStates = next.threadComponentStates
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

func (r memoryChats) ListIDs(ctx context.Context) ([]modeluuid.UUID, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	ids := make([]modeluuid.UUID, 0, len(r.s.chats))
	for _, chat := range r.s.chats {
		if chat.ID.IsNull() {
			continue
		}
		ids = append(ids, chat.ID)
	}
	return ids, nil
}

type memoryThreads struct{ s *MemoryStorage }

func (r memoryThreads) Save(ctx context.Context, thread *coremodel.Thread) error {
	_ = ctx
	if thread == nil {
		return nil
	}
	thread.Label = strings.TrimSpace(thread.Label)
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

func (r memoryThreads) ListIDs(ctx context.Context) ([]modeluuid.UUID, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	return memoryThreadIDs(r.s.threads), nil
}

func memoryThreadIDs(threads map[modeluuid.UUID]coremodel.Thread) []modeluuid.UUID {
	ids := make([]modeluuid.UUID, 0, len(threads))
	for id := range threads {
		ids = append(ids, id)
	}
	return ids
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

func (r memoryChatComponents) FindByComponentRoleAndExternalChannelID(ctx context.Context, componentID modeluuid.UUID, role coremodel.ChatComponentRole, externalChannelID string) (*coremodel.ChatComponent, error) {
	_ = ctx
	externalChannelID = strings.TrimSpace(externalChannelID)
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, binding := range r.s.chatComponents {
		if binding.ComponentID == componentID && binding.Role == role && strings.TrimSpace(binding.ExternalChannelID) == externalChannelID && binding.Enabled {
			copy := binding
			return &copy, nil
		}
	}
	return nil, nil
}

type memoryInboundFilterBindings struct{ s *MemoryStorage }

func (r memoryInboundFilterBindings) Save(ctx context.Context, binding *coremodel.InboundFilterBinding) error {
	_ = ctx
	if binding == nil {
		return nil
	}
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	now := time.Now()
	if binding.ID.IsNull() {
		for id, existing := range r.s.inboundFilterBindings {
			if existing.SourceBindingID == binding.SourceBindingID && existing.FilterComponentID == binding.FilterComponentID {
				binding.ID = id
				if binding.CreatedAt.IsZero() {
					binding.CreatedAt = existing.CreatedAt
				}
				break
			}
		}
	}
	if binding.ID.IsNull() {
		binding.ID = modeluuid.New()
		binding.CreatedAt = now
	} else if existing, ok := r.s.inboundFilterBindings[binding.ID]; ok && binding.CreatedAt.IsZero() {
		binding.CreatedAt = existing.CreatedAt
	}
	binding.UpdatedAt = now
	r.s.inboundFilterBindings[binding.ID] = *binding
	return nil
}

func (r memoryInboundFilterBindings) GetBySourceBindingAndFilter(ctx context.Context, sourceBindingID modeluuid.UUID, filterComponentID modeluuid.UUID) (*coremodel.InboundFilterBinding, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, binding := range r.s.inboundFilterBindings {
		if binding.SourceBindingID == sourceBindingID && binding.FilterComponentID == filterComponentID {
			copy := binding
			return &copy, nil
		}
	}
	return nil, nil
}

func (r memoryInboundFilterBindings) ListEnabledBySourceBindingID(ctx context.Context, sourceBindingID modeluuid.UUID) ([]coremodel.InboundFilterBinding, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []coremodel.InboundFilterBinding
	for _, binding := range r.s.inboundFilterBindings {
		if binding.SourceBindingID == sourceBindingID && binding.Enabled {
			out = append(out, binding)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

type memoryInboundDrops struct{ s *MemoryStorage }

func (r memoryInboundDrops) Save(ctx context.Context, drop *coremodel.InboundDrop) error {
	_ = ctx
	if drop == nil {
		return nil
	}
	drop.ExternalChannelID = strings.TrimSpace(drop.ExternalChannelID)
	drop.ExternalThreadID = strings.TrimSpace(drop.ExternalThreadID)
	drop.ChatLabel = strings.TrimSpace(drop.ChatLabel)
	drop.ActorID = strings.TrimSpace(drop.ActorID)
	drop.ActorLabel = strings.TrimSpace(drop.ActorLabel)
	drop.LastTextPreview = strings.TrimSpace(drop.LastTextPreview)
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	now := time.Now()
	if drop.ID.IsNull() {
		for id, existing := range r.s.inboundDrops {
			if existing.ComponentID == drop.ComponentID && strings.TrimSpace(existing.ExternalChannelID) == drop.ExternalChannelID {
				drop.ID = id
				if drop.FirstSeenAt.IsZero() {
					drop.FirstSeenAt = existing.FirstSeenAt
				}
				break
			}
		}
	}
	if drop.ID.IsNull() {
		drop.ID = modeluuid.New()
		if drop.FirstSeenAt.IsZero() {
			drop.FirstSeenAt = now
		}
	} else if existing, ok := r.s.inboundDrops[drop.ID]; ok && drop.FirstSeenAt.IsZero() {
		drop.FirstSeenAt = existing.FirstSeenAt
	}
	if drop.LastSeenAt.IsZero() {
		drop.LastSeenAt = now
	}
	r.s.inboundDrops[drop.ID] = *drop
	return nil
}

func (r memoryInboundDrops) GetByComponentAndExternalChannelID(ctx context.Context, componentID modeluuid.UUID, externalChannelID string) (*coremodel.InboundDrop, error) {
	_ = ctx
	externalChannelID = strings.TrimSpace(externalChannelID)
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, drop := range r.s.inboundDrops {
		if drop.ComponentID == componentID && strings.TrimSpace(drop.ExternalChannelID) == externalChannelID {
			copy := drop
			return &copy, nil
		}
	}
	return nil, nil
}

func (r memoryInboundDrops) List(ctx context.Context) ([]coremodel.InboundDrop, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	out := make([]coremodel.InboundDrop, 0, len(r.s.inboundDrops))
	for _, drop := range r.s.inboundDrops {
		out = append(out, drop)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LastSeenAt.Equal(out[j].LastSeenAt) {
			return out[i].FirstSeenAt.After(out[j].FirstSeenAt)
		}
		return out[i].LastSeenAt.After(out[j].LastSeenAt)
	})
	return out, nil
}

func (r memoryInboundDrops) DeleteByComponentAndExternalChannelID(ctx context.Context, componentID modeluuid.UUID, externalChannelID string) error {
	_ = ctx
	externalChannelID = strings.TrimSpace(externalChannelID)
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for id, drop := range r.s.inboundDrops {
		if drop.ComponentID == componentID && strings.TrimSpace(drop.ExternalChannelID) == externalChannelID {
			delete(r.s.inboundDrops, id)
		}
	}
	return nil
}

type memoryDroppedEvents struct{ s *MemoryStorage }

func (r memoryDroppedEvents) Save(ctx context.Context, event *coremodel.DroppedEvent) error {
	_ = ctx
	if event == nil {
		return nil
	}
	event.ProviderChannelID = strings.TrimSpace(event.ProviderChannelID)
	event.ProviderThreadID = strings.TrimSpace(event.ProviderThreadID)
	event.ProviderMessageID = strings.TrimSpace(event.ProviderMessageID)
	event.SenderKey = strings.TrimSpace(event.SenderKey)
	event.SenderLabel = strings.TrimSpace(event.SenderLabel)
	event.Subject = strings.TrimSpace(event.Subject)
	event.Preview = strings.TrimSpace(event.Preview)
	now := time.Now()
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	}
	if event.ExpiresAt.IsZero() {
		event.ExpiresAt = event.CreatedAt.Add(30 * 24 * time.Hour)
	}
	if event.ID.IsNull() {
		event.ID = modeluuid.New()
	}
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	r.s.droppedEvents[event.ID] = *event
	return nil
}

func (r memoryDroppedEvents) GetByID(ctx context.Context, eventID modeluuid.UUID) (*coremodel.DroppedEvent, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	event, ok := r.s.droppedEvents[eventID]
	if !ok {
		return nil, nil
	}
	return &event, nil
}

func (r memoryDroppedEvents) ListIDs(ctx context.Context) ([]modeluuid.UUID, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	ids := make([]modeluuid.UUID, 0, len(r.s.droppedEvents))
	for id := range r.s.droppedEvents {
		if id.IsNull() {
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (r memoryDroppedEvents) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var deleted int64
	for id, event := range r.s.droppedEvents {
		if !event.ExpiresAt.IsZero() && !event.ExpiresAt.After(now) {
			delete(r.s.droppedEvents, id)
			deleted++
		}
	}
	return deleted, nil
}

type memoryAllowlistSenders struct{ s *MemoryStorage }

func (r memoryAllowlistSenders) Save(ctx context.Context, sender *coremodel.AllowlistSender) error {
	_ = ctx
	if sender == nil {
		return nil
	}
	sender.SenderKey = strings.TrimSpace(sender.SenderKey)
	sender.SenderLabel = strings.TrimSpace(sender.SenderLabel)
	now := time.Now()
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if sender.ID.IsNull() {
		for id, existing := range r.s.allowlistSenders {
			if existing.SourceBindingID == sender.SourceBindingID && strings.TrimSpace(existing.SenderKey) == sender.SenderKey {
				sender.ID = id
				if sender.CreatedAt.IsZero() {
					sender.CreatedAt = existing.CreatedAt
				}
				break
			}
		}
	}
	if sender.ID.IsNull() {
		sender.ID = modeluuid.New()
		sender.CreatedAt = now
	} else if existing, ok := r.s.allowlistSenders[sender.ID]; ok && sender.CreatedAt.IsZero() {
		sender.CreatedAt = existing.CreatedAt
	}
	sender.UpdatedAt = now
	r.s.allowlistSenders[sender.ID] = *sender
	return nil
}

func (r memoryAllowlistSenders) GetBySourceBindingAndSenderKey(ctx context.Context, sourceBindingID modeluuid.UUID, senderKey string) (*coremodel.AllowlistSender, error) {
	_ = ctx
	senderKey = strings.TrimSpace(senderKey)
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, sender := range r.s.allowlistSenders {
		if sender.SourceBindingID == sourceBindingID && strings.TrimSpace(sender.SenderKey) == senderKey {
			copy := sender
			return &copy, nil
		}
	}
	return nil, nil
}

func (r memoryAllowlistSenders) ListBySourceBindingID(ctx context.Context, sourceBindingID modeluuid.UUID) ([]coremodel.AllowlistSender, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	out := make([]coremodel.AllowlistSender, 0, len(r.s.allowlistSenders))
	for _, sender := range r.s.allowlistSenders {
		if sender.SourceBindingID == sourceBindingID {
			out = append(out, sender)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].SenderKey < out[j].SenderKey
	})
	return out, nil
}

func (r memoryAllowlistSenders) DeleteBySourceBindingAndSenderKey(ctx context.Context, sourceBindingID modeluuid.UUID, senderKey string) (bool, error) {
	_ = ctx
	senderKey = strings.TrimSpace(senderKey)
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	deleted := false
	for id, sender := range r.s.allowlistSenders {
		if sender.SourceBindingID == sourceBindingID && strings.TrimSpace(sender.SenderKey) == senderKey {
			delete(r.s.allowlistSenders, id)
			deleted = true
		}
	}
	return deleted, nil
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
	} else if existing, ok := r.s.threadComponentMappings[mapping.ID]; ok && mapping.CreatedAt.IsZero() {
		mapping.CreatedAt = existing.CreatedAt
	}
	mapping.UpdatedAt = now
	r.s.threadComponentMappings[mapping.ID] = *mapping
	return nil
}

func (r memoryThreadMappings) GetByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) (*coremodel.ThreadComponentMapping, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, mapping := range r.s.threadComponentMappings {
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
	for _, mapping := range r.s.threadComponentMappings {
		if mapping.ChatID == chatID && mapping.ComponentID == componentID && strings.TrimSpace(mapping.ComponentThreadID) == componentThreadID {
			copy := mapping
			return &copy, nil
		}
	}
	return nil, nil
}

func (r memoryThreadMappings) ListByChatID(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.ThreadComponentMapping, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	out := make([]coremodel.ThreadComponentMapping, 0)
	for _, mapping := range r.s.threadComponentMappings {
		if mapping.ChatID == chatID {
			out = append(out, mapping)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID.String() < out[j].ID.String()
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (r memoryThreadMappings) DeleteByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) error {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for id, mapping := range r.s.threadComponentMappings {
		if mapping.ThreadID == threadID && mapping.ComponentID == componentID {
			delete(r.s.threadComponentMappings, id)
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
		for id, existing := range r.s.threadComponentStates {
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
	} else if existing, ok := r.s.threadComponentStates[state.ID]; ok && state.CreatedAt.IsZero() {
		state.CreatedAt = existing.CreatedAt
	}
	state.UpdatedAt = now
	r.s.threadComponentStates[state.ID] = *state
	return nil
}

func (r memoryThreadStates) GetByThreadAndComponent(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) (*coremodel.ThreadComponentState, error) {
	_ = ctx
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, state := range r.s.threadComponentStates {
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
	for id, state := range r.s.threadComponentStates {
		if state.ThreadID == threadID && state.ComponentID == componentID {
			delete(r.s.threadComponentStates, id)
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
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID.String() < out[j].ID.String()
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
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
