package broker

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/messaging"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

var _ messaging.Service = (*Broker)(nil)

func (b *Broker) ListThreads(ctx context.Context, actor coremodel.Actor, req messaging.ListThreadsRequest) ([]messaging.ThreadSummary, error) {
	if err := b.ensureReady(); err != nil {
		return nil, err
	}
	if err := requireMessagingActor(actor); err != nil {
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

	threads, err := b.threadSummaries(ctx, true)
	if err != nil {
		return nil, err
	}
	if query != "" {
		filtered := make([]messaging.ThreadSummary, 0, len(threads))
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

func (b *Broker) ListMessages(ctx context.Context, actor coremodel.Actor, threadID modeluuid.UUID, req messaging.ListMessagesRequest) (messaging.MessagePage, error) {
	if err := b.ensureReady(); err != nil {
		return messaging.MessagePage{}, err
	}
	if err := requireMessagingActor(actor); err != nil {
		return messaging.MessagePage{}, err
	}
	thread, chat, err := b.loadThreadAndChat(ctx, threadID)
	if err != nil {
		return messaging.MessagePage{}, err
	}
	if !chat.Enabled {
		return messaging.MessagePage{}, fmt.Errorf("thread chat is disabled: %s", chat.ID)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	messages, err := b.Storage.Messages().ListByThreadID(ctx, thread.ID)
	if err != nil {
		return messaging.MessagePage{}, err
	}
	if len(messages) == 0 {
		return messaging.MessagePage{}, nil
	}

	cursor := strings.TrimSpace(req.Cursor)
	start := 0
	if cursor == "" {
		if len(messages) > limit {
			start = len(messages) - limit
		}
		return messaging.MessagePage{
			Messages: cloneThreadMessages(messages[start:]),
		}, nil
	}

	index, err := resolveMessageCursor(messages, cursor)
	if err != nil {
		return messaging.MessagePage{}, err
	}
	start = index + 1
	if start >= len(messages) {
		return messaging.MessagePage{Messages: []coremodel.ThreadMessage{}}, nil
	}
	end := start + limit
	if end > len(messages) {
		end = len(messages)
	}
	page := messaging.MessagePage{
		Messages: cloneThreadMessages(messages[start:end]),
	}
	if end < len(messages) && len(page.Messages) > 0 {
		page.NextCursor = page.Messages[len(page.Messages)-1].ID.String()
	}
	return page, nil
}

func (b *Broker) SendMessage(ctx context.Context, actor coremodel.Actor, threadID modeluuid.UUID, req messaging.SendMessageRequest) (*messaging.SendMessageResult, error) {
	if err := b.ensureReady(); err != nil {
		return nil, err
	}
	actor = actor.Resolved()
	if err := requireMessagingActor(actor); err != nil {
		return nil, err
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return nil, fmt.Errorf("missing message")
	}

	targetThread, targetChat, err := b.loadThreadAndChat(ctx, threadID)
	if err != nil {
		return nil, err
	}
	if !targetChat.Enabled {
		return nil, fmt.Errorf("target chat is disabled: %s", targetChat.ID)
	}
	if !req.SourceThreadID.IsNull() && req.SourceThreadID == targetThread.ID {
		return nil, fmt.Errorf("cannot send thread message to the current thread")
	}

	var result *messaging.SendMessageResult
	if err := b.Turns.Run(ctx, targetThread.ID, func() error {
		var runErr error
		result, runErr = b.sendMessagingThreadMessage(ctx, actor, *targetThread, *targetChat, req)
		return runErr
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (b *Broker) ResolveThreadRef(ctx context.Context, ref string) (modeluuid.UUID, error) {
	if err := b.ensureReady(); err != nil {
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
		thread, err := b.Storage.Threads().GetByID(ctx, parsed)
		if err != nil {
			return modeluuid.Nil, err
		}
		if thread != nil {
			return parsed, nil
		}
	}

	threads, err := b.threadSummaries(ctx, false)
	if err != nil {
		return modeluuid.Nil, err
	}
	var matches []messaging.ThreadSummary
	for _, thread := range threads {
		if strings.HasPrefix(thread.ID.String(), ref) {
			matches = append(matches, thread)
		}
	}
	switch len(matches) {
	case 0:
		return modeluuid.Nil, fmt.Errorf("thread not found: %s", ref)
	case 1:
		return matches[0].ID, nil
	default:
		return modeluuid.Nil, ambiguousThreadRefError(ref, matches)
	}
}

func (b *Broker) ActorForThread(ctx context.Context, threadID modeluuid.UUID) (coremodel.Actor, error) {
	thread, chat, err := b.loadThreadAndChat(ctx, threadID)
	if err != nil {
		return coremodel.Actor{}, err
	}
	return coremodel.Actor{
		ID:    "thread:" + thread.ID.String(),
		Label: interThreadSourceLabel(*chat, *thread),
	}, nil
}

func (b *Broker) sendMessagingThreadMessage(
	ctx context.Context,
	actor coremodel.Actor,
	targetThread coremodel.Thread,
	targetChat coremodel.Chat,
	req messaging.SendMessageRequest,
) (*messaging.SendMessageResult, error) {
	runtime, err := b.runtimeForChat(ctx, targetChat)
	if err != nil {
		return nil, err
	}

	inbound := &coremodel.ThreadMessage{
		ChatID:      targetChat.ID,
		ThreadID:    targetThread.ID,
		Direction:   coremodel.MessageDirectionInbound,
		Kind:        coremodel.MessageKindUser,
		ActorID:     strings.TrimSpace(actor.ID),
		ActorLabel:  strings.TrimSpace(actor.Label),
		Text:        strings.TrimSpace(req.Text),
		ExternalID:  strings.TrimSpace(req.ExternalID),
		ComponentID: req.ComponentID,
	}
	if req.SourceThreadID.IsNull() {
		inbound.MetadataJSON = "provider=thread"
	} else {
		inbound.MetadataJSON = strings.Join([]string{
			"provider=thread",
			"source_thread_id=" + req.SourceThreadID.String(),
		}, "\n")
	}
	if err := b.Storage.Messages().Append(ctx, inbound); err != nil {
		return nil, err
	}
	if _, err := b.runStoredThreadTurn(ctx, runtime, targetChat, targetThread, *inbound); err != nil {
		return nil, err
	}
	return &messaging.SendMessageResult{Message: *inbound}, nil
}

func (b *Broker) threadSummaries(ctx context.Context, activeOnly bool) ([]messaging.ThreadSummary, error) {
	chats, err := b.Storage.Chats().List(ctx)
	if err != nil {
		return nil, err
	}
	var out []messaging.ThreadSummary
	for _, chat := range chats {
		if !chat.Enabled {
			continue
		}
		threads, err := b.Storage.Threads().ListByChatID(ctx, chat.ID)
		if err != nil {
			return nil, err
		}
		for _, thread := range threads {
			messages, err := b.Storage.Messages().ListByThreadID(ctx, thread.ID)
			if err != nil {
				return nil, err
			}
			if activeOnly && len(messages) == 0 {
				continue
			}
			summary := messaging.ThreadSummary{
				ID:          thread.ID,
				ChatID:      chat.ID,
				ChatLabel:   chat.Label,
				ThreadLabel: thread.Label,
			}
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

func sortThreadSummaries(threads []messaging.ThreadSummary) {
	sort.SliceStable(threads, func(i, j int) bool {
		if !threads[i].LastMessageAt.Equal(threads[j].LastMessageAt) {
			return threads[i].LastMessageAt.After(threads[j].LastMessageAt)
		}
		return threads[i].ID.String() < threads[j].ID.String()
	})
}

func ambiguousThreadRefError(ref string, matches []messaging.ThreadSummary) error {
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

func (b *Broker) loadThreadAndChat(ctx context.Context, threadID modeluuid.UUID) (*coremodel.Thread, *coremodel.Chat, error) {
	thread, err := b.Storage.Threads().GetByID(ctx, threadID)
	if err != nil {
		return nil, nil, err
	}
	if thread == nil {
		return nil, nil, fmt.Errorf("thread not found: %s", threadID)
	}
	chat, err := b.Storage.Chats().GetByID(ctx, thread.ChatID)
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

func requireMessagingActor(actor coremodel.Actor) error {
	actor = actor.Resolved()
	if strings.TrimSpace(actor.ID) == "" || strings.TrimSpace(actor.Label) == "" {
		return fmt.Errorf("missing actor identity")
	}
	return nil
}

func threadMatchesQuery(thread messaging.ThreadSummary, query string) bool {
	if query == "" {
		return true
	}
	values := []string{
		thread.ID.String(),
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
