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
	switch {
	case scope.All:
		if !scope.ChatID.IsNull() || !scope.ThreadID.IsNull() {
			return fmt.Errorf("message scope --all cannot be combined with chat or thread")
		}
		chats, err := s.Storage.Chats().List(ctx)
		if err != nil {
			return err
		}
		sort.SliceStable(chats, func(i, j int) bool {
			if chats[i].CreatedAt.Equal(chats[j].CreatedAt) {
				return chats[i].ID.String() < chats[j].ID.String()
			}
			return chats[i].CreatedAt.Before(chats[j].CreatedAt)
		})
		for _, chat := range chats {
			if err := s.forEachChatMessage(ctx, chat.ID, visit); err != nil {
				return err
			}
		}
		return nil
	case !scope.ThreadID.IsNull():
		if !scope.ChatID.IsNull() {
			return fmt.Errorf("message scope chat and thread are mutually exclusive")
		}
		messages, err := s.Storage.Messages().ListByThreadID(ctx, scope.ThreadID)
		if err != nil {
			return err
		}
		for _, message := range messages {
			if err := visit(message); err != nil {
				return err
			}
		}
		return nil
	case !scope.ChatID.IsNull():
		return s.forEachChatMessage(ctx, scope.ChatID, visit)
	default:
		return fmt.Errorf("missing message scope")
	}
}

func (s *service) forEachChatMessage(ctx context.Context, chatID modeluuid.UUID, visit component.MessageVisitor) error {
	threads, err := s.Storage.Threads().ListByChatID(ctx, chatID)
	if err != nil {
		return err
	}
	sort.SliceStable(threads, func(i, j int) bool {
		if threads[i].CreatedAt.Equal(threads[j].CreatedAt) {
			return threads[i].ID.String() < threads[j].ID.String()
		}
		return threads[i].CreatedAt.Before(threads[j].CreatedAt)
	})
	for _, thread := range threads {
		messages, err := s.Storage.Messages().ListByThreadID(ctx, thread.ID)
		if err != nil {
			return err
		}
		for _, message := range messages {
			if err := visit(message); err != nil {
				return err
			}
		}
	}
	return nil
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
