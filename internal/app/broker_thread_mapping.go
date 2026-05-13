package app

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/threadmapping"
)

func (s *service) EnsureThread(ctx context.Context, binding coremodel.ChatComponent, componentThreadID string) (*coremodel.Thread, error) {
	return threadmapping.New(s.Repository()).EnsureThread(ctx, binding, componentThreadID)
}

func (s *service) ComponentThreadID(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) (string, bool, error) {
	return threadmapping.New(s.Repository()).ComponentThreadID(ctx, threadID, componentID)
}

func (s *service) BindComponentThreadID(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID, componentThreadID string) error {
	return threadmapping.New(s.Repository()).BindComponentThreadID(ctx, threadID, componentID, componentThreadID)
}

func (s *service) RelayTarget(ctx context.Context, threadID modeluuid.UUID, binding coremodel.ChatComponent) (*message.ChatTarget, bool, error) {
	return threadmapping.New(s.Repository()).RelayTarget(ctx, threadID, binding)
}
