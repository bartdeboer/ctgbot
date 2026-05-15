package app

import (
	"context"
	"fmt"

	broker "github.com/bartdeboer/ctgbot/internal/broker"
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

func (s *service) RuntimeSpec(ctx context.Context, chat coremodel.Chat) (broker.RuntimeSpec, error) {
	if s == nil || s.Storage == nil {
		return broker.RuntimeSpec{}, fmt.Errorf("missing app storage")
	}
	workspace, err := s.ResolveChatWorkspace(ctx, chat)
	if err != nil {
		return broker.RuntimeSpec{}, err
	}
	bindings, err := s.Storage.ChatComponents().ListEnabledByChatID(ctx, chat.ID)
	if err != nil {
		return broker.RuntimeSpec{}, err
	}
	loaded := make(map[modeluuid.UUID]*component.Loaded)
	for _, binding := range bindings {
		if _, ok := loaded[binding.ComponentID]; ok {
			continue
		}
		instance, err := s.ResolveComponent(ctx, binding.ComponentID)
		if err != nil {
			return broker.RuntimeSpec{}, err
		}
		loaded[binding.ComponentID] = instance
	}
	return broker.RuntimeSpec{Chat: chat, Workspace: workspace, Bindings: bindings, Loaded: loaded}, nil
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
