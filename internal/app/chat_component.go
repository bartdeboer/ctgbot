package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

type ChatBindResult struct {
	Chat      coremodel.Chat
	Component coremodel.Component
	Bindings  []coremodel.ChatComponent
}

type ChatComponentAddResult struct {
	Binding      coremodel.ChatComponent
	ComponentRef string
	Runtime      string
	ProfilePath  string
}

type ChatComponentRemoveResult struct {
	Binding      coremodel.ChatComponent
	ComponentRef string
	Removed      bool
}

type ChatComponentInfo struct {
	Binding      coremodel.ChatComponent
	ComponentRef string
	Runtime      string
}

func (s *service) BindInboundChat(ctx context.Context, componentRef string, externalChannelID string, label string, roleFlag string) (ChatBindResult, error) {
	if s == nil || s.Storage == nil {
		return ChatBindResult{}, fmt.Errorf("missing app storage")
	}
	registration, err := s.resolveComponentRegistration(ctx, componentRef)
	if err != nil {
		return ChatBindResult{}, err
	}
	loaded, err := s.resolveLoadedComponent(ctx, registration.ID)
	if err != nil {
		return ChatBindResult{}, err
	}
	roles, err := resolveChatBindRoles(loaded, roleFlag)
	if err != nil {
		return ChatBindResult{}, err
	}

	externalChannelID = strings.TrimSpace(externalChannelID)
	if externalChannelID == "" {
		return ChatBindResult{}, fmt.Errorf("missing external channel id")
	}
	label = strings.TrimSpace(label)
	drop, err := s.Storage.InboundDrops().GetByComponentAndExternalChannelID(ctx, registration.ID, externalChannelID)
	if err != nil {
		return ChatBindResult{}, err
	}
	if label == "" && drop != nil {
		label = strings.TrimSpace(drop.ChatLabel)
	}
	if label == "" {
		label = externalChannelID
	}

	chat, bindings, err := s.createInboundChatBinding(ctx, *registration, externalChannelID, label, roles)
	if err != nil {
		return ChatBindResult{}, err
	}
	return ChatBindResult{Chat: *chat, Component: *registration, Bindings: bindings}, nil
}

func (s *service) AddChatComponent(ctx context.Context, chatID modeluuid.UUID, role coremodel.ChatComponentRole, componentRef string, externalChannelID string) (ChatComponentAddResult, error) {
	if s == nil || s.Storage == nil {
		return ChatComponentAddResult{}, fmt.Errorf("missing app storage")
	}
	registration, err := s.resolveComponentRegistration(ctx, componentRef)
	if err != nil {
		return ChatComponentAddResult{}, err
	}

	externalChannelID = strings.TrimSpace(externalChannelID)
	if role == coremodel.ChatComponentRoleCommand {
		loaded, err := s.resolveLoadedComponent(ctx, registration.ID)
		if err != nil {
			return ChatComponentAddResult{}, err
		}
		if _, ok := loaded.Component.(component.CommandSurface); !ok {
			return ChatComponentAddResult{}, fmt.Errorf("component %s does not support command chat bindings", registration.Ref())
		}
	}
	if externalChannelID == "" && role == coremodel.ChatComponentRoleSource {
		externalChannelID, err = s.defaultSourceExternalChannelID(ctx, registration.ID)
		if err != nil {
			return ChatComponentAddResult{}, err
		}
	}
	binding, err := s.bindChatComponent(ctx, chatID, role, *registration, externalChannelID)
	if err != nil {
		return ChatComponentAddResult{}, err
	}
	return ChatComponentAddResult{
		Binding:      *binding,
		ComponentRef: registration.Ref(),
		Runtime:      registration.Runtime,
		ProfilePath:  registration.ProfilePath,
	}, nil
}

func (s *service) RemoveChatComponent(ctx context.Context, chatID modeluuid.UUID, role coremodel.ChatComponentRole, componentRef string) (ChatComponentRemoveResult, error) {
	if s == nil || s.Storage == nil {
		return ChatComponentRemoveResult{}, fmt.Errorf("missing app storage")
	}
	if chatID.IsNull() {
		return ChatComponentRemoveResult{}, fmt.Errorf("missing chat id")
	}
	registration, err := s.resolveComponentRegistration(ctx, componentRef)
	if err != nil {
		return ChatComponentRemoveResult{}, err
	}
	binding, err := s.Storage.ChatComponents().GetByChatComponentRole(ctx, chatID, registration.ID, role)
	if err != nil {
		return ChatComponentRemoveResult{}, err
	}
	if binding == nil || !binding.Enabled {
		return ChatComponentRemoveResult{ComponentRef: registration.Ref()}, nil
	}
	binding.Enabled = false
	if err := s.Storage.ChatComponents().Save(ctx, binding); err != nil {
		return ChatComponentRemoveResult{}, err
	}
	return ChatComponentRemoveResult{Binding: *binding, ComponentRef: registration.Ref(), Removed: true}, nil
}

func (s *service) ListChatComponents(ctx context.Context, chatID modeluuid.UUID) ([]ChatComponentInfo, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing app storage")
	}
	if chatID.IsNull() {
		return nil, fmt.Errorf("missing chat id")
	}
	bindings, err := s.Storage.ChatComponents().ListEnabledByChatID(ctx, chatID)
	if err != nil {
		return nil, err
	}
	out := make([]ChatComponentInfo, 0, len(bindings))
	for _, binding := range bindings {
		ref := binding.ComponentID.String()
		runtimeKind := ""
		registration, err := s.Storage.Components().GetByID(ctx, binding.ComponentID)
		if err != nil {
			return nil, err
		}
		if registration != nil {
			ref = registration.Ref()
			runtimeKind = registration.Runtime
		}
		out = append(out, ChatComponentInfo{
			Binding:      binding,
			ComponentRef: ref,
			Runtime:      runtimeKind,
		})
	}
	return out, nil
}

func (s *service) defaultSourceExternalChannelID(ctx context.Context, componentID modeluuid.UUID) (string, error) {
	loaded, err := s.resolveLoadedComponent(ctx, componentID)
	if err != nil {
		return "", err
	}
	defaults, ok := loaded.Component.(component.SourceBindingDefaults)
	if !ok {
		return "", nil
	}
	return defaults.DefaultSourceExternalChannelID(ctx)
}

func resolveChatBindRoles(loaded *component.Loaded, roleFlag string) ([]coremodel.ChatComponentRole, error) {
	if loaded == nil || loaded.Component == nil {
		return nil, fmt.Errorf("missing loaded component")
	}
	_, hasSource := loaded.Component.(component.InboundSource)
	_, hasRelay := loaded.Component.(component.OutboundRelay)

	switch strings.TrimSpace(roleFlag) {
	case "", "auto":
		switch {
		case hasSource && hasRelay:
			return []coremodel.ChatComponentRole{coremodel.ChatComponentRoleSource, coremodel.ChatComponentRoleRelay}, nil
		case hasSource:
			return []coremodel.ChatComponentRole{coremodel.ChatComponentRoleSource}, nil
		case hasRelay:
			return []coremodel.ChatComponentRole{coremodel.ChatComponentRoleRelay}, nil
		default:
			return nil, fmt.Errorf("component %s does not support source or relay chat bindings", loaded.Registration.Ref())
		}
	case string(coremodel.ChatComponentRoleSource):
		if !hasSource {
			return nil, fmt.Errorf("component %s does not support source chat bindings", loaded.Registration.Ref())
		}
		return []coremodel.ChatComponentRole{coremodel.ChatComponentRoleSource}, nil
	case string(coremodel.ChatComponentRoleRelay):
		if !hasRelay {
			return nil, fmt.Errorf("component %s does not support relay chat bindings", loaded.Registration.Ref())
		}
		return []coremodel.ChatComponentRole{coremodel.ChatComponentRoleRelay}, nil
	case "all":
		if !hasSource || !hasRelay {
			return nil, fmt.Errorf("component %s does not support binding both source and relay roles", loaded.Registration.Ref())
		}
		return []coremodel.ChatComponentRole{coremodel.ChatComponentRoleSource, coremodel.ChatComponentRoleRelay}, nil
	default:
		return nil, fmt.Errorf("invalid bind role %q", roleFlag)
	}
}

func (s *service) createInboundChatBinding(ctx context.Context, registration coremodel.Component, externalChannelID string, label string, roles []coremodel.ChatComponentRole) (*coremodel.Chat, []coremodel.ChatComponent, error) {
	if s == nil || s.Storage == nil {
		return nil, nil, fmt.Errorf("missing app storage")
	}
	externalChannelID = strings.TrimSpace(externalChannelID)
	label = strings.TrimSpace(label)
	if externalChannelID == "" {
		return nil, nil, fmt.Errorf("missing external channel id")
	}
	if label == "" {
		label = externalChannelID
	}
	if len(roles) == 0 {
		return nil, nil, fmt.Errorf("missing chat bind roles")
	}

	var chat coremodel.Chat
	var bindings []coremodel.ChatComponent
	err := s.Storage.Transaction(ctx, func(tx repository.Storage) error {
		for _, role := range roles {
			existing, err := tx.ChatComponents().FindByComponentRoleAndExternalChannelID(ctx, registration.ID, role, externalChannelID)
			if err != nil {
				return err
			}
			if existing != nil {
				return fmt.Errorf("external channel %q is already bound to chat %s as %s", externalChannelID, existing.ChatID, role)
			}
		}

		chat = coremodel.Chat{
			Label:   label,
			Enabled: true,
		}
		if err := tx.Chats().Save(ctx, &chat); err != nil {
			return err
		}
		bindings = make([]coremodel.ChatComponent, 0, len(roles))
		for _, role := range roles {
			binding := coremodel.ChatComponent{
				ChatID:            chat.ID,
				ComponentID:       registration.ID,
				Role:              role,
				ExternalChannelID: externalChannelID,
				Enabled:           true,
			}
			if err := tx.ChatComponents().Save(ctx, &binding); err != nil {
				return err
			}
			bindings = append(bindings, binding)
		}
		return tx.InboundDrops().DeleteByComponentAndExternalChannelID(ctx, registration.ID, externalChannelID)
	})
	if err != nil {
		return nil, nil, err
	}
	return &chat, bindings, nil
}

func (s *service) bindChatComponent(ctx context.Context, chatID modeluuid.UUID, role coremodel.ChatComponentRole, registration coremodel.Component, externalChannelID string) (*coremodel.ChatComponent, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing app storage")
	}
	if chatID.IsNull() {
		return nil, fmt.Errorf("missing chat id")
	}
	if !role.Valid() {
		return nil, fmt.Errorf("invalid chat component role: %q", role)
	}

	chat, err := s.Storage.Chats().GetByID(ctx, chatID)
	if err != nil {
		return nil, err
	}
	if chat == nil {
		return nil, fmt.Errorf("chat not found: %s", chatID)
	}

	externalChannelID = strings.TrimSpace(externalChannelID)
	switch role {
	case coremodel.ChatComponentRoleSource, coremodel.ChatComponentRoleRelay:
		if externalChannelID == "" {
			return nil, fmt.Errorf("missing external channel id for role %q", role)
		}
	default:
		externalChannelID = ""
	}

	binding, err := s.Storage.ChatComponents().GetByChatComponentRole(ctx, chatID, registration.ID, role)
	if err != nil {
		return nil, err
	}
	if binding == nil {
		binding = &coremodel.ChatComponent{
			ChatID:      chatID,
			ComponentID: registration.ID,
			Role:        role,
		}
	}
	binding.ExternalChannelID = externalChannelID
	binding.Enabled = true
	if err := s.Storage.ChatComponents().Save(ctx, binding); err != nil {
		return nil, err
	}
	return binding, nil
}
