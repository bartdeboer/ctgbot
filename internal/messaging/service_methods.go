package messaging

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func (s *Service) ListThreads(ctx context.Context, actor coremodel.Actor, req ListThreadsRequest) ([]ThreadSummary, error) {
	if err := s.ensureStorage(); err != nil {
		return nil, err
	}
	if err := requireActor(actor); err != nil {
		return nil, err
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	query := strings.ToLower(strings.TrimSpace(req.Query))

	threads, err := s.threadSummaries(ctx, true)
	if err != nil {
		return nil, err
	}
	if query != "" {
		filtered := make([]ThreadSummary, 0, len(threads))
		for _, thread := range threads {
			if threadMatchesQuery(thread, query) {
				filtered = append(filtered, thread)
			}
		}
		threads = filtered
	}
	sortThreadSummaries(threads)
	if len(threads) > limit {
		threads = threads[:limit]
	}
	return threads, nil
}

func (s *Service) ListMessages(ctx context.Context, actor coremodel.Actor, threadID modeluuid.UUID, req ListMessagesRequest) (MessagePage, error) {
	if err := s.ensureStorage(); err != nil {
		return MessagePage{}, err
	}
	if err := requireActor(actor); err != nil {
		return MessagePage{}, err
	}
	thread, chat, err := s.loadThreadAndChat(ctx, threadID)
	if err != nil {
		return MessagePage{}, err
	}
	if !chat.Enabled {
		return MessagePage{}, fmt.Errorf("thread chat is disabled: %s", chat.ID)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	messages, err := s.Storage.Messages().ListByThreadID(ctx, thread.ID)
	if err != nil {
		return MessagePage{}, err
	}
	if len(messages) == 0 {
		return MessagePage{}, nil
	}

	cursor := strings.TrimSpace(req.Cursor)
	start := 0
	if cursor == "" {
		if len(messages) > limit {
			start = len(messages) - limit
		}
		return MessagePage{
			Messages: cloneThreadMessages(messages[start:]),
		}, nil
	}

	index, err := resolveMessageCursor(messages, cursor)
	if err != nil {
		return MessagePage{}, err
	}
	start = index + 1
	if start >= len(messages) {
		return MessagePage{Messages: []coremodel.ThreadMessage{}}, nil
	}
	end := start + limit
	if end > len(messages) {
		end = len(messages)
	}
	page := MessagePage{
		Messages: cloneThreadMessages(messages[start:end]),
	}
	if end < len(messages) && len(page.Messages) > 0 {
		page.NextCursor = page.Messages[len(page.Messages)-1].ID.String()
	}
	return page, nil
}

func (s *Service) PurgeThread(ctx context.Context, actor coremodel.Actor, threadID modeluuid.UUID) (PurgeThreadResult, error) {
	if err := s.ensureStorage(); err != nil {
		return PurgeThreadResult{}, err
	}
	if err := requireActor(actor); err != nil {
		return PurgeThreadResult{}, err
	}
	if !actor.HasRole(simplerbac.RoleRoot) && !actor.HasRole(simplerbac.RoleUser) {
		return PurgeThreadResult{}, fmt.Errorf("purge thread denied: missing role")
	}
	thread, chat, err := s.loadThreadAndChat(ctx, threadID)
	if err != nil {
		return PurgeThreadResult{}, err
	}
	if !chat.Enabled {
		return PurgeThreadResult{}, fmt.Errorf("thread chat is disabled: %s", chat.ID)
	}
	result := PurgeThreadResult{ThreadID: thread.ID}
	if err := s.Storage.Transaction(ctx, func(tx repository.Storage) error {
		agentMappingsDeleted, err := purgeAgentThreadMappings(ctx, tx, chat.ID, thread.ID)
		if err != nil {
			return err
		}
		artifactsDeleted, err := tx.Artifacts().DeleteByThreadID(ctx, thread.ID)
		if err != nil {
			return err
		}
		messagesDeleted, err := tx.Messages().DeleteByThreadID(ctx, thread.ID)
		if err != nil {
			return err
		}
		result.MessagesDeleted = messagesDeleted
		result.ArtifactsDeleted = artifactsDeleted
		result.AgentMappingsDeleted = agentMappingsDeleted
		return nil
	}); err != nil {
		return PurgeThreadResult{}, err
	}
	return result, nil
}

func purgeAgentThreadMappings(ctx context.Context, storage repository.Storage, chatID modeluuid.UUID, threadID modeluuid.UUID) (int64, error) {
	bindings, err := storage.ChatComponents().ListEnabledByChatID(ctx, chatID)
	if err != nil {
		return 0, err
	}
	var deleted int64
	seen := map[modeluuid.UUID]bool{}
	for _, binding := range bindings {
		if binding.Role != coremodel.ChatComponentRoleAgent || seen[binding.ComponentID] {
			continue
		}
		seen[binding.ComponentID] = true
		mapping, err := storage.ThreadComponentMappings().GetByThreadAndComponent(ctx, threadID, binding.ComponentID)
		if err != nil {
			return 0, err
		}
		if mapping == nil {
			continue
		}
		if err := storage.ThreadComponentMappings().DeleteByThreadAndComponent(ctx, threadID, binding.ComponentID); err != nil {
			return 0, err
		}
		deleted++
	}
	return deleted, nil
}

func (s *Service) ThreadStatus(ctx context.Context, actor coremodel.Actor, threadID modeluuid.UUID) (ThreadStatus, error) {
	if err := s.ensureStorage(); err != nil {
		return ThreadStatus{}, err
	}
	if err := requireActor(actor); err != nil {
		return ThreadStatus{}, err
	}
	thread, chat, err := s.loadThreadAndChat(ctx, threadID)
	if err != nil {
		return ThreadStatus{}, err
	}
	threadResolver, err := s.threadShortIDResolver(ctx)
	if err != nil {
		return ThreadStatus{}, err
	}
	chatResolver, err := s.chatShortIDResolver(ctx)
	if err != nil {
		return ThreadStatus{}, err
	}
	components, err := s.threadStatusComponents(ctx, *chat, *thread)
	if err != nil {
		return ThreadStatus{}, err
	}
	return ThreadStatus{
		ID:          thread.ID,
		ShortID:     threadShortID(threadResolver, thread.ID),
		Label:       strings.TrimSpace(thread.Label),
		ChatID:      chat.ID,
		ChatShortID: chatShortID(chatResolver, chat.ID),
		ChatLabel:   strings.TrimSpace(chat.Label),
		ChatEnabled: chat.Enabled,
		Components:  components,
	}, nil
}

func (s *Service) SetThreadLabel(ctx context.Context, actor coremodel.Actor, threadID modeluuid.UUID, label string) (ThreadStatus, error) {
	if err := s.ensureStorage(); err != nil {
		return ThreadStatus{}, err
	}
	if err := requireActor(actor); err != nil {
		return ThreadStatus{}, err
	}
	label = strings.TrimSpace(label)
	if label == "" {
		return ThreadStatus{}, fmt.Errorf("missing thread label")
	}
	if err := s.Storage.Transaction(ctx, func(tx repository.Storage) error {
		thread, err := tx.Threads().GetByID(ctx, threadID)
		if err != nil {
			return err
		}
		if thread == nil {
			return fmt.Errorf("thread not found: %s", threadID)
		}
		thread.Label = label
		return tx.Threads().Save(ctx, thread)
	}); err != nil {
		return ThreadStatus{}, err
	}
	return s.ThreadStatus(ctx, actor, threadID)
}

func (s *Service) BindThreadComponent(ctx context.Context, actor coremodel.Actor, threadID modeluuid.UUID, req ThreadComponentBindRequest) (ThreadComponentBindResult, error) {
	if err := s.ensureStorage(); err != nil {
		return ThreadComponentBindResult{}, err
	}
	if err := requireActor(actor); err != nil {
		return ThreadComponentBindResult{}, err
	}
	thread, chat, err := s.loadThreadAndChat(ctx, threadID)
	if err != nil {
		return ThreadComponentBindResult{}, err
	}
	registration, err := s.resolveComponentRef(ctx, req.ComponentRef)
	if err != nil {
		return ThreadComponentBindResult{}, err
	}
	providerThreadID := strings.TrimSpace(req.ProviderThreadID)
	if providerThreadID == "" {
		providerThreadID, err = s.inferProviderThreadID(ctx, *chat, *registration)
		if err != nil {
			return ThreadComponentBindResult{}, err
		}
	}

	existing, err := s.Storage.ThreadComponentMappings().FindByChatComponentAndThreadID(ctx, chat.ID, registration.ID, providerThreadID)
	if err != nil {
		return ThreadComponentBindResult{}, err
	}
	if existing != nil {
		if existing.ThreadID == thread.ID {
			return ThreadComponentBindResult{
				ThreadID:         thread.ID,
				ComponentRef:     registration.Ref(),
				ProviderThreadID: providerThreadID,
			}, nil
		}
		return ThreadComponentBindResult{}, fmt.Errorf("component %s provider thread %q is already bound to thread %s; choose another providerThreadID or repair the existing mapping first", registration.Ref(), providerThreadID, s.threadRefWithShortID(ctx, existing.ThreadID))
	}

	mapping, err := s.Storage.ThreadComponentMappings().GetByThreadAndComponent(ctx, thread.ID, registration.ID)
	if err != nil {
		return ThreadComponentBindResult{}, err
	}
	if mapping != nil {
		currentProviderThreadID := strings.TrimSpace(mapping.ComponentThreadID)
		if currentProviderThreadID == providerThreadID {
			return ThreadComponentBindResult{
				ThreadID:         thread.ID,
				ComponentRef:     registration.Ref(),
				ProviderThreadID: providerThreadID,
			}, nil
		}
		return ThreadComponentBindResult{}, fmt.Errorf("component %s is already bound on this thread to provider thread %q; repair the existing mapping first", registration.Ref(), currentProviderThreadID)
	}
	mapping = &coremodel.ThreadComponentMapping{
		ThreadID:    thread.ID,
		ChatID:      chat.ID,
		ComponentID: registration.ID,
	}
	mapping.ChatID = chat.ID
	mapping.ComponentThreadID = providerThreadID
	if err := s.Storage.ThreadComponentMappings().Save(ctx, mapping); err != nil {
		return ThreadComponentBindResult{}, err
	}
	return ThreadComponentBindResult{
		ThreadID:         thread.ID,
		ComponentRef:     registration.Ref(),
		ProviderThreadID: providerThreadID,
	}, nil
}

func (s *Service) ResolveThreadRef(ctx context.Context, ref string) (modeluuid.UUID, error) {
	if err := s.ensureStorage(); err != nil {
		return modeluuid.Nil, err
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return modeluuid.Nil, fmt.Errorf("missing thread id")
	}
	if strings.EqualFold(ref, "current") {
		return modeluuid.Nil, fmt.Errorf("current thread ref requires command context")
	}
	if parsed, err := modeluuid.Parse(ref); err == nil {
		thread, err := s.Storage.Threads().GetByID(ctx, parsed)
		if err != nil {
			return modeluuid.Nil, err
		}
		if thread != nil {
			return parsed, nil
		}
	}

	resolver, err := s.threadShortIDResolver(ctx)
	if err != nil {
		return modeluuid.Nil, err
	}
	threadID, err := resolver.Resolve(ref)
	if err == nil {
		return threadID, nil
	}
	var ambiguous *repository.ShortIDAmbiguousError
	if errors.As(err, &ambiguous) {
		return modeluuid.Nil, s.ambiguousThreadRefError(ctx, ref, ambiguous.Candidates)
	}
	var notFound *repository.ShortIDNotFoundError
	if errors.As(err, &notFound) {
		return modeluuid.Nil, fmt.Errorf("thread not found: %s", ref)
	}
	if err != nil {
		return modeluuid.Nil, err
	}
	return modeluuid.Nil, fmt.Errorf("thread not found: %s", ref)
}

func (s *Service) resolveComponentRef(ctx context.Context, ref string) (*coremodel.Component, error) {
	parsed, err := coremodel.ParseComponentRef(ref)
	if err != nil {
		return nil, err
	}
	if !parsed.ExplicitName {
		registration, err := s.Storage.Components().GetDefaultByType(ctx, parsed.Type)
		if err != nil {
			return nil, err
		}
		if registration != nil {
			return registration, nil
		}
	}
	registration, err := s.Storage.Components().GetByTypeAndName(ctx, parsed.Type, parsed.ResolvedName())
	if err != nil {
		return nil, err
	}
	if registration == nil {
		return nil, fmt.Errorf("component not registered: %s", parsed.Ref())
	}
	return registration, nil
}

func (s *Service) inferProviderThreadID(ctx context.Context, chat coremodel.Chat, registration coremodel.Component) (string, error) {
	bindings, err := s.Storage.ChatComponents().ListEnabledByChatID(ctx, chat.ID)
	if err != nil {
		return "", err
	}
	var matches []string
	seen := map[string]bool{}
	for _, binding := range bindings {
		if binding.ComponentID != registration.ID || binding.Role != coremodel.ChatComponentRoleSource {
			continue
		}
		externalChannelID := strings.TrimSpace(binding.ExternalChannelID)
		if externalChannelID != "" && !seen[externalChannelID] {
			seen[externalChannelID] = true
			matches = append(matches, externalChannelID)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("cannot infer provider thread id for %s in chat %s; pass providerThreadID explicitly", registration.Ref(), chat.ID)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("provider thread id for %s in chat %s is ambiguous; pass providerThreadID explicitly", registration.Ref(), chat.ID)
	}
}

func (s *Service) threadRefWithShortID(ctx context.Context, threadID modeluuid.UUID) string {
	if threadID.IsNull() {
		return threadID.String()
	}
	out := threadID.String()
	resolver, err := s.threadShortIDResolver(ctx)
	if err != nil {
		return out
	}
	shortID, err := resolver.ShortIDFor(threadID, 6)
	if err != nil || strings.TrimSpace(shortID) == "" || shortID == out {
		return out
	}
	return out + " (short_id: " + shortID + ")"
}

func (s *Service) ActorForThread(ctx context.Context, threadID modeluuid.UUID) (coremodel.Actor, error) {
	thread, chat, err := s.loadThreadAndChat(ctx, threadID)
	if err != nil {
		return coremodel.Actor{}, err
	}
	return coremodel.Actor{
		ID:    "thread:" + thread.ID.String(),
		Label: interThreadSourceLabel(*chat, *thread),
	}, nil
}

func (s *Service) ThreadTarget(ctx context.Context, threadID modeluuid.UUID) (*coremodel.Chat, *coremodel.Thread, error) {
	thread, chat, err := s.loadThreadAndChat(ctx, threadID)
	if err != nil {
		return nil, nil, err
	}
	return chat, thread, nil
}

func (s *Service) ensureStorage() error {
	if s == nil || s.Storage == nil {
		return fmt.Errorf("missing messaging storage")
	}
	return nil
}

func requireActor(actor coremodel.Actor) error {
	actor = ResolveActor(actor)
	if strings.TrimSpace(actor.ID) == "" || strings.TrimSpace(actor.Label) == "" {
		return fmt.Errorf("missing actor identity")
	}
	return nil
}

func (s *Service) threadSummaries(ctx context.Context, activeOnly bool) ([]ThreadSummary, error) {
	resolver, err := s.threadShortIDResolver(ctx)
	if err != nil {
		return nil, err
	}
	chats, err := s.Storage.Chats().List(ctx)
	if err != nil {
		return nil, err
	}
	var out []ThreadSummary
	for _, chat := range chats {
		if !chat.Enabled {
			continue
		}
		threads, err := s.Storage.Threads().ListByChatID(ctx, chat.ID)
		if err != nil {
			return nil, err
		}
		for _, thread := range threads {
			messages, err := s.Storage.Messages().ListByThreadID(ctx, thread.ID)
			if err != nil {
				return nil, err
			}
			if activeOnly && len(messages) == 0 {
				continue
			}
			summary := ThreadSummary{
				ID:          thread.ID,
				ChatID:      chat.ID,
				ChatLabel:   chat.Label,
				ThreadLabel: thread.Label,
			}
			summary.ShortID = threadShortID(resolver, thread.ID)
			if len(messages) > 0 {
				last := messages[len(messages)-1]
				summary.LastMessageAt = last.CreatedAt
				summary.LastMessageText = last.Text
			} else {
				summary.LastMessageAt = thread.UpdatedAt
			}
			out = append(out, summary)
		}
	}
	return out, nil
}

func sortThreadSummaries(threads []ThreadSummary) {
	sort.SliceStable(threads, func(i, j int) bool {
		if !threads[i].LastMessageAt.Equal(threads[j].LastMessageAt) {
			return threads[i].LastMessageAt.After(threads[j].LastMessageAt)
		}
		return threads[i].ID.String() < threads[j].ID.String()
	})
}

func (s *Service) ambiguousThreadRefError(ctx context.Context, ref string, candidates []modeluuid.UUID) error {
	threads, err := s.threadSummaries(ctx, false)
	if err != nil {
		return err
	}
	byID := map[modeluuid.UUID]ThreadSummary{}
	for _, thread := range threads {
		byID[thread.ID] = thread
	}
	matches := make([]ThreadSummary, 0, len(candidates))
	for _, candidate := range candidates {
		summary, ok := byID[candidate]
		if !ok {
			summary = ThreadSummary{ID: candidate, ShortID: candidate.String()}
		}
		matches = append(matches, summary)
	}
	sortThreadSummaries(matches)
	lines := []string{
		fmt.Sprintf("short thread ID %s is ambiguous", ref),
		"hint: The candidates are:",
	}
	for _, match := range matches {
		label := strings.TrimSpace(match.ChatLabel)
		if match.ThreadLabel != "" {
			if label != "" {
				label += " / "
			}
			label += match.ThreadLabel
		}
		if label == "" {
			label = match.ChatID.String()
		}
		lines = append(lines, fmt.Sprintf("  %s thread %s", match.ID.String(), label))
	}
	return errors.New(strings.Join(lines, "\n"))
}

func (s *Service) threadShortIDResolver(ctx context.Context) (*repository.ShortIDResolver, error) {
	ids, err := s.Storage.Threads().ListIDs(ctx)
	if err != nil {
		return nil, err
	}
	return repository.NewShortIDResolver(ids), nil
}

func (s *Service) chatShortIDResolver(ctx context.Context) (*repository.ShortIDResolver, error) {
	ids, err := s.Storage.Chats().ListIDs(ctx)
	if err != nil {
		return nil, err
	}
	return repository.NewShortIDResolver(ids), nil
}

func (s *Service) threadStatusComponents(ctx context.Context, chat coremodel.Chat, thread coremodel.Thread) ([]ThreadStatusComponent, error) {
	bindings, err := s.Storage.ChatComponents().ListEnabledByChatID(ctx, chat.ID)
	if err != nil {
		return nil, err
	}
	out := make([]ThreadStatusComponent, 0, len(bindings))
	for _, binding := range bindings {
		componentRef := binding.ComponentID.String()
		registration, err := s.Storage.Components().GetByID(ctx, binding.ComponentID)
		if err != nil {
			return nil, err
		}
		if registration != nil {
			componentRef = registration.Ref()
		}
		statusComponent := ThreadStatusComponent{
			Ref:               componentRef,
			Role:              string(binding.Role),
			ExternalChannelID: strings.TrimSpace(binding.ExternalChannelID),
		}
		mapping, err := s.Storage.ThreadComponentMappings().GetByThreadAndComponent(ctx, thread.ID, binding.ComponentID)
		if err != nil {
			return nil, err
		}
		if mapping != nil {
			statusComponent.ExternalThreadID = strings.TrimSpace(mapping.ComponentThreadID)
		}
		out = append(out, statusComponent)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ref != out[j].Ref {
			return out[i].Ref < out[j].Ref
		}
		return out[i].Role < out[j].Role
	})
	return out, nil
}

func threadShortID(resolver *repository.ShortIDResolver, threadID modeluuid.UUID) string {
	shortID, err := resolver.ShortIDFor(threadID, 6)
	if err != nil {
		return threadID.String()
	}
	return shortID
}

func chatShortID(resolver *repository.ShortIDResolver, chatID modeluuid.UUID) string {
	shortID, err := resolver.ShortIDFor(chatID, 6)
	if err != nil {
		return chatID.String()
	}
	return shortID
}

func (s *Service) loadThreadAndChat(ctx context.Context, threadID modeluuid.UUID) (*coremodel.Thread, *coremodel.Chat, error) {
	thread, err := s.Storage.Threads().GetByID(ctx, threadID)
	if err != nil {
		return nil, nil, err
	}
	if thread == nil {
		return nil, nil, fmt.Errorf("thread not found: %s", threadID)
	}
	chat, err := s.Storage.Chats().GetByID(ctx, thread.ChatID)
	if err != nil {
		return nil, nil, err
	}
	if chat == nil {
		return nil, nil, fmt.Errorf("chat not found: %s", thread.ChatID)
	}
	return thread, chat, nil
}

func interThreadSourceLabel(chat coremodel.Chat, thread coremodel.Thread) string {
	label := strings.TrimSpace(chat.Label)
	threadLabel := strings.TrimSpace(thread.Label)
	if threadLabel != "" {
		if label != "" {
			label += " / "
		}
		label += threadLabel
	}
	if label == "" {
		label = thread.ID.String()
	}
	return label
}

func threadMatchesQuery(thread ThreadSummary, query string) bool {
	if query == "" {
		return true
	}
	values := []string{
		thread.ID.String(),
		strings.ToLower(strings.TrimSpace(thread.ShortID)),
		strings.ToLower(strings.TrimSpace(thread.ChatLabel)),
		strings.ToLower(strings.TrimSpace(thread.ThreadLabel)),
		strings.ToLower(strings.TrimSpace(thread.LastMessageText)),
	}
	for _, value := range values {
		if strings.Contains(value, query) {
			return true
		}
	}
	return false
}

func cloneThreadMessages(messages []coremodel.ThreadMessage) []coremodel.ThreadMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]coremodel.ThreadMessage, len(messages))
	copy(out, messages)
	return out
}

func resolveMessageCursor(messages []coremodel.ThreadMessage, cursor string) (int, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return -1, fmt.Errorf("missing cursor")
	}
	if parsed, err := modeluuid.Parse(cursor); err == nil {
		for i, message := range messages {
			if message.ID == parsed {
				return i, nil
			}
		}
	}
	var matches []int
	for i, message := range messages {
		if strings.HasPrefix(message.ID.String(), cursor) {
			matches = append(matches, i)
		}
	}
	switch len(matches) {
	case 0:
		return -1, fmt.Errorf("message cursor not found: %s", cursor)
	case 1:
		return matches[0], nil
	default:
		return -1, fmt.Errorf("message cursor is ambiguous: %s", cursor)
	}
}
