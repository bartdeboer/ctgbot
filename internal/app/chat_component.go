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
	HomePath     string
}

type ChatComponentInfo struct {
	Binding      coremodel.ChatComponent
	ComponentRef string
	Runtime      string
}

func (s *Service) BindInboundChat(ctx context.Context, componentRef string, externalChatID string, label string, roleFlag string) (ChatBindResult, error) {
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

	externalChatID = strings.TrimSpace(externalChatID)
	if externalChatID == "" {
		return ChatBindResult{}, fmt.Errorf("missing external chat id")
	}
	label = strings.TrimSpace(label)
	drop, err := s.Storage.InboundDrops().GetByComponentAndExternalChatID(ctx, registration.ID, externalChatID)
	if err != nil {
		return ChatBindResult{}, err
	}
	if label == "" && drop != nil {
		label = strings.TrimSpace(drop.ChatLabel)
	}
	if label == "" {
		label = externalChatID
	}

	chat, bindings, err := s.createInboundChatBinding(ctx, *registration, externalChatID, label, roles)
	if err != nil {
		return ChatBindResult{}, err
	}
	return ChatBindResult{Chat: *chat, Component: *registration, Bindings: bindings}, nil
}

func (s *Service) AddChatComponent(ctx context.Context, chatID modeluuid.UUID, role coremodel.ChatComponentRole, componentRef string, externalChatID string) (ChatComponentAddResult, error) {
	if s == nil || s.Storage == nil {
		return ChatComponentAddResult{}, fmt.Errorf("missing app storage")
	}
	registration, err := s.resolveComponentRegistration(ctx, componentRef)
	if err != nil {
		return ChatComponentAddResult{}, err
	}

	externalChatID = strings.TrimSpace(externalChatID)
	if externalChatID == "" && role == coremodel.ChatComponentRoleSource {
		externalChatID, err = s.defaultSourceExternalChatID(ctx, registration.ID)
		if err != nil {
			return ChatComponentAddResult{}, err
		}
	}
	binding, err := s.bindChatComponent(ctx, chatID, role, *registration, externalChatID)
	if err != nil {
		return ChatComponentAddResult{}, err
	}
	return ChatComponentAddResult{
		Binding:      *binding,
		ComponentRef: registration.Ref(),
		Runtime:      registration.Runtime,
		HomePath:     registration.HomePath,
	}, nil
}

func (s *Service) ListChatComponents(ctx context.Context, chatID modeluuid.UUID) ([]ChatComponentInfo, error) {
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

func (s *Service) defaultSourceExternalChatID(ctx context.Context, componentID modeluuid.UUID) (string, error) {
	loaded, err := s.resolveLoadedComponent(ctx, componentID)
	if err != nil {
		return "", err
	}
	defaults, ok := loaded.Component.(component.SourceBindingDefaults)
	if !ok {
		return "", nil
	}
	return defaults.DefaultSourceExternalChatID(ctx)
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

func (s *Service) createInboundChatBinding(ctx context.Context, registration coremodel.Component, externalChatID string, label string, roles []coremodel.ChatComponentRole) (*coremodel.Chat, []coremodel.ChatComponent, error) {
	if s == nil || s.Storage == nil {
		return nil, nil, fmt.Errorf("missing app storage")
	}
	externalChatID = strings.TrimSpace(externalChatID)
	label = strings.TrimSpace(label)
	if externalChatID == "" {
		return nil, nil, fmt.Errorf("missing external chat id")
	}
	if label == "" {
		label = externalChatID
	}
	if len(roles) == 0 {
		return nil, nil, fmt.Errorf("missing chat bind roles")
	}

	var chat coremodel.Chat
	var bindings []coremodel.ChatComponent
	err := s.Storage.Transaction(ctx, func(tx repository.Storage) error {
		for _, role := range roles {
			existing, err := tx.ChatComponents().FindByComponentRoleAndExternalChatID(ctx, registration.ID, role, externalChatID)
			if err != nil {
				return err
			}
			if existing != nil {
				return fmt.Errorf("external chat %q is already bound to chat %s as %s", externalChatID, existing.ChatID, role)
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
				ChatID:         chat.ID,
				ComponentID:    registration.ID,
				Role:           role,
				ExternalChatID: externalChatID,
				Enabled:        true,
			}
			if err := tx.ChatComponents().Save(ctx, &binding); err != nil {
				return err
			}
			bindings = append(bindings, binding)
		}
		return tx.InboundDrops().DeleteByComponentAndExternalChatID(ctx, registration.ID, externalChatID)
	})
	if err != nil {
		return nil, nil, err
	}
	return &chat, bindings, nil
}

func (s *Service) bindChatComponent(ctx context.Context, chatID modeluuid.UUID, role coremodel.ChatComponentRole, registration coremodel.Component, externalChatID string) (*coremodel.ChatComponent, error) {
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

	externalChatID = strings.TrimSpace(externalChatID)
	switch role {
	case coremodel.ChatComponentRoleSource, coremodel.ChatComponentRoleRelay:
		if externalChatID == "" {
			return nil, fmt.Errorf("missing external chat id for role %q", role)
		}
	default:
		externalChatID = ""
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
	binding.ExternalChatID = externalChatID
	binding.Enabled = true
	if err := s.Storage.ChatComponents().Save(ctx, binding); err != nil {
		return nil, err
	}
	return binding, nil
}
