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

func threadShortID(resolver *repository.ShortIDResolver, threadID modeluuid.UUID) string {
	shortID, err := resolver.ShortIDFor(threadID, 6)
	if err != nil {
		return threadID.String()
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
