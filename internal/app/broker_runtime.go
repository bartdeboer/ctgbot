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

func (s *service) ChatMessages(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.ThreadMessage, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing app storage")
	}
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
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID.String() < out[j].ID.String()
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
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
