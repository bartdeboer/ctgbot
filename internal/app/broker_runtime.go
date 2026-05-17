package app

import (
	"context"
	"fmt"
	"sort"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func (s *service) Chat(ctx context.Context, chatID modeluuid.UUID) (*coremodel.Chat, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing app storage")
	}
	return s.Storage.Chats().GetByID(ctx, chatID)
}

func (s *service) Thread(ctx context.Context, threadID modeluuid.UUID) (*coremodel.Thread, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing app storage")
	}
	return s.Storage.Threads().GetByID(ctx, threadID)
}

func (s *service) ThreadMessages(ctx context.Context, threadID modeluuid.UUID) ([]coremodel.ThreadMessage, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing app storage")
	}
	return s.Storage.Messages().ListByThreadID(ctx, threadID)
}

func (s *service) ForEachMessage(ctx context.Context, scope component.MessageScope, visit component.MessageVisitor) error {
	if s == nil || s.Storage == nil {
		return fmt.Errorf("missing app storage")
	}
	if visit == nil {
		return fmt.Errorf("missing message visitor")
	}
	var messages []coremodel.ThreadMessage
	switch {
	case scope.All:
		if !scope.ChatID.IsNull() || !scope.ThreadID.IsNull() {
			return fmt.Errorf("message scope --all cannot be combined with chat or thread")
		}
		chats, err := s.Storage.Chats().List(ctx)
		if err != nil {
			return err
		}
		for _, chat := range chats {
			chatMessages, err := s.chatMessages(ctx, chat.ID)
			if err != nil {
				return err
			}
			messages = append(messages, chatMessages...)
		}
	case !scope.ThreadID.IsNull():
		if !scope.ChatID.IsNull() {
			return fmt.Errorf("message scope chat and thread are mutually exclusive")
		}
		var err error
		messages, err = s.Storage.Messages().ListByThreadID(ctx, scope.ThreadID)
		if err != nil {
			return err
		}
	case !scope.ChatID.IsNull():
		var err error
		messages, err = s.chatMessages(ctx, scope.ChatID)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("missing message scope")
	}
	messages = filterMessages(messages, scope)
	sortMessages(messages, scope.Order)
	if scope.Limit > 0 && len(messages) > scope.Limit {
		messages = messages[:scope.Limit]
	}
	for _, message := range messages {
		if err := visit(message); err != nil {
			return err
		}
	}
	return nil
}

func filterMessages(messages []coremodel.ThreadMessage, scope component.MessageScope) []coremodel.ThreadMessage {
	if len(scope.Kinds) == 0 {
		return messages
	}
	out := messages[:0]
	for _, message := range messages {
		if messageScopeAllows(scope, message) {
			out = append(out, message)
		}
	}
	return out
}

func messageScopeAllows(scope component.MessageScope, message coremodel.ThreadMessage) bool {
	if len(scope.Kinds) == 0 {
		return true
	}
	for _, kind := range scope.Kinds {
		if message.Kind == kind {
			return true
		}
	}
	return false
}

func (s *service) chatMessages(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.ThreadMessage, error) {
	threads, err := s.Storage.Threads().ListByChatID(ctx, chatID)
	if err != nil {
		return nil, err
	}
	var out []coremodel.ThreadMessage
	for _, thread := range threads {
		messages, err := s.Storage.Messages().ListByThreadID(ctx, thread.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, messages...)
	}
	return out, nil
}

func sortMessages(messages []coremodel.ThreadMessage, order component.MessageOrder) {
	sort.SliceStable(messages, func(i, j int) bool {
		if messages[i].CreatedAt.Equal(messages[j].CreatedAt) {
			if order == component.MessageOrderNewestFirst {
				return messages[i].ID.String() > messages[j].ID.String()
			}
			return messages[i].ID.String() < messages[j].ID.String()
		}
		if order == component.MessageOrderNewestFirst {
			return messages[i].CreatedAt.After(messages[j].CreatedAt)
		}
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})
}

func (s *service) EnabledChatComponents(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.ChatComponent, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing app storage")
	}
	return s.Storage.ChatComponents().ListEnabledByChatID(ctx, chatID)
}

func (s *service) EnabledInboundSources(ctx context.Context) ([]component.InboundSource, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing app storage")
	}
	components, err := s.Storage.Components().ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	sources := make([]component.InboundSource, 0, len(components))
	for _, registration := range components {
		instance, err := s.ResolveComponent(ctx, registration.ID)
		if err != nil {
			return nil, err
		}
		if source, ok := instance.Component.(component.InboundSource); ok {
			sources = append(sources, source)
		}
	}
	return sources, nil
}
