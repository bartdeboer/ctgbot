package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type InboundDropInfo struct {
	ComponentRef    string
	ExternalChatID  string
	MessageCount    int64
	LastSeenAt      time.Time
	ChatLabel       string
	ActorLabel      string
	ActorID         string
	LastTextPreview string
}

func (s *Service) CreateChat(ctx context.Context, label string, workspace string) (coremodel.Chat, error) {
	if s == nil || s.Storage == nil {
		return coremodel.Chat{}, fmt.Errorf("missing app storage")
	}
	label = strings.TrimSpace(label)
	workspace = strings.TrimSpace(workspace)
	if label == "" {
		return coremodel.Chat{}, fmt.Errorf("missing chat label")
	}
	if workspace != "" {
		if err := s.validateWorkspace(workspace); err != nil {
			return coremodel.Chat{}, err
		}
	}
	chat := coremodel.Chat{
		Label:     label,
		Workspace: workspace,
		Enabled:   true,
	}
	if err := s.Storage.Chats().Save(ctx, &chat); err != nil {
		return coremodel.Chat{}, err
	}
	return chat, nil
}

func (s *Service) ListChats(ctx context.Context) ([]coremodel.Chat, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing app storage")
	}
	return s.Storage.Chats().List(ctx)
}

func (s *Service) ListInboundDrops(ctx context.Context) ([]InboundDropInfo, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing app storage")
	}
	drops, err := s.Storage.InboundDrops().List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]InboundDropInfo, 0, len(drops))
	for _, drop := range drops {
		ref := drop.ComponentID.String()
		registration, err := s.Storage.Components().GetByID(ctx, drop.ComponentID)
		if err != nil {
			return nil, err
		}
		if registration != nil {
			ref = registration.Ref()
		}
		out = append(out, InboundDropInfo{
			ComponentRef:    ref,
			ExternalChatID:  drop.ExternalChatID,
			MessageCount:    drop.MessageCount,
			LastSeenAt:      drop.LastSeenAt,
			ChatLabel:       drop.ChatLabel,
			ActorLabel:      drop.ActorLabel,
			ActorID:         drop.ActorID,
			LastTextPreview: drop.LastTextPreview,
		})
	}
	return out, nil
}

func (s *Service) SetChatWorkspace(ctx context.Context, chatID modeluuid.UUID, workspace string) (coremodel.Chat, error) {
	if s == nil || s.Storage == nil {
		return coremodel.Chat{}, fmt.Errorf("missing app storage")
	}
	if chatID.IsNull() {
		return coremodel.Chat{}, fmt.Errorf("missing chat id")
	}
	chat, err := s.Storage.Chats().GetByID(ctx, chatID)
	if err != nil {
		return coremodel.Chat{}, err
	}
	if chat == nil {
		return coremodel.Chat{}, fmt.Errorf("chat not found: %s", chatID)
	}
	workspace = strings.TrimSpace(workspace)
	if workspace != "" {
		if err := s.validateWorkspace(workspace); err != nil {
			return coremodel.Chat{}, err
		}
	}
	chat.Workspace = workspace
	if err := s.Storage.Chats().Save(ctx, chat); err != nil {
		return coremodel.Chat{}, err
	}
	return *chat, nil
}

func (s *Service) validateWorkspace(workspace string) error {
	if s == nil || s.WorkspaceValidator == nil || strings.TrimSpace(workspace) == "" {
		return nil
	}
	return s.WorkspaceValidator.ValidateWorkspace(workspace)
}
