package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

type ChatInfo struct {
	Chat    coremodel.Chat
	ShortID string
}

type InboundDropInfo struct {
	ComponentRef      string
	ExternalChannelID string
	MessageCount      int64
	LastSeenAt        time.Time
	ChatLabel         string
	ActorLabel        string
	ActorID           string
	LastTextPreview   string
}

func (s *service) CreateChat(ctx context.Context, label string, workspace string) (coremodel.Chat, error) {
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

func (s *service) ListChats(ctx context.Context) ([]ChatInfo, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing app storage")
	}
	chats, err := s.Storage.Chats().List(ctx)
	if err != nil {
		return nil, err
	}
	resolver, err := s.chatShortIDResolver(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ChatInfo, 0, len(chats))
	for _, chat := range chats {
		out = append(out, ChatInfo{
			Chat:    chat,
			ShortID: chatShortID(resolver, chat.ID),
		})
	}
	return out, nil
}

func (s *service) ResolveChatRef(ctx context.Context, ref string) (modeluuid.UUID, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return modeluuid.Nil, fmt.Errorf("missing chat id")
	}
	resolver, err := s.chatShortIDResolver(ctx)
	if err != nil {
		return modeluuid.Nil, err
	}
	chatID, err := resolver.Resolve(ref)
	if err == nil {
		return chatID, nil
	}
	var ambiguous *repository.ShortIDAmbiguousError
	if errors.As(err, &ambiguous) {
		return modeluuid.Nil, s.ambiguousChatRefError(ctx, ref, resolver, ambiguous.Candidates)
	}
	var notFound *repository.ShortIDNotFoundError
	if errors.As(err, &notFound) {
		return modeluuid.Nil, fmt.Errorf("chat not found: %s", ref)
	}
	return modeluuid.Nil, err
}

func (s *service) ListInboundDrops(ctx context.Context) ([]InboundDropInfo, error) {
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
			ComponentRef:      ref,
			ExternalChannelID: drop.ExternalChannelID,
			MessageCount:      drop.MessageCount,
			LastSeenAt:        drop.LastSeenAt,
			ChatLabel:         drop.ChatLabel,
			ActorLabel:        drop.ActorLabel,
			ActorID:           drop.ActorID,
			LastTextPreview:   drop.LastTextPreview,
		})
	}
	return out, nil
}

func (s *service) SetChatWorkspace(ctx context.Context, chatID modeluuid.UUID, workspace string) (coremodel.Chat, error) {
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

func (s *service) validateWorkspace(workspace string) error {
	if s == nil || s.WorkspaceValidator == nil || strings.TrimSpace(workspace) == "" {
		return nil
	}
	return s.WorkspaceValidator.ValidateWorkspace(workspace)
}

func (s *service) chatShortIDResolver(ctx context.Context) (*repository.ShortIDResolver, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing app storage")
	}
	ids, err := s.Storage.Chats().ListIDs(ctx)
	if err != nil {
		return nil, err
	}
	return repository.NewShortIDResolver(ids), nil
}

func (s *service) ambiguousChatRefError(ctx context.Context, ref string, resolver *repository.ShortIDResolver, candidates []modeluuid.UUID) error {
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].String() < candidates[j].String()
	})
	lines := []string{
		fmt.Sprintf("chat id %s is ambiguous", strings.TrimSpace(ref)),
		"candidates:",
	}
	for _, candidate := range candidates {
		label := ""
		chat, err := s.Storage.Chats().GetByID(ctx, candidate)
		if err == nil && chat != nil {
			label = strings.TrimSpace(chat.Label)
		}
		line := fmt.Sprintf("- %s %s", chatShortID(resolver, candidate), candidate.String())
		if label != "" {
			line += " " + label
		}
		lines = append(lines, line)
	}
	return errors.New(strings.Join(lines, "\n"))
}

func chatShortID(resolver *repository.ShortIDResolver, chatID modeluuid.UUID) string {
	shortID, err := resolver.ShortIDFor(chatID, 6)
	if err != nil {
		return chatID.String()
	}
	return shortID
}
