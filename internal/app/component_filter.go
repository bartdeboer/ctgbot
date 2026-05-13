package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/inbound"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

type ComponentFilterAddResult struct {
	Chat          coremodel.Chat
	Source        coremodel.Component
	SourceBinding coremodel.ChatComponent
	Filter        coremodel.Component
	Binding       coremodel.InboundFilterBinding
}

type ComponentFilterRemoveResult struct {
	Chat          coremodel.Chat
	Source        coremodel.Component
	SourceBinding coremodel.ChatComponent
	Filter        coremodel.Component
	Disabled      bool
}

type ComponentFilterClearResult struct {
	Chat          coremodel.Chat
	Source        coremodel.Component
	SourceBinding coremodel.ChatComponent
	Disabled      int
}

type ComponentFilterListResult struct {
	Chat          coremodel.Chat
	Source        coremodel.Component
	SourceBinding coremodel.ChatComponent
	Bindings      []ComponentFilterListBinding
}

type ComponentFilterListBinding struct {
	Binding   coremodel.InboundFilterBinding
	FilterRef string
}

func (s *service) AddChatComponentFilter(ctx context.Context, chatRef string, sourceRef string, externalChannelID string, filterRef string) (ComponentFilterAddResult, error) {
	if s == nil || s.Storage == nil {
		return ComponentFilterAddResult{}, fmt.Errorf("missing app storage")
	}
	chat, source, sourceBinding, err := s.resolveChatSourceBinding(ctx, chatRef, sourceRef, externalChannelID)
	if err != nil {
		return ComponentFilterAddResult{}, err
	}
	filter, err := s.resolveInboundEventFilterRegistration(ctx, filterRef)
	if err != nil {
		return ComponentFilterAddResult{}, err
	}

	var binding coremodel.InboundFilterBinding
	if err := s.Storage.Transaction(ctx, func(tx repository.Storage) error {
		current, err := tx.InboundFilterBindings().GetBySourceBindingAndFilter(ctx, sourceBinding.ID, filter.ID)
		if err != nil {
			return err
		}
		if current != nil {
			binding = *current
		} else {
			binding = coremodel.InboundFilterBinding{
				SourceBindingID:   sourceBinding.ID,
				FilterComponentID: filter.ID,
			}
		}
		binding.Enabled = true
		return tx.InboundFilterBindings().Save(ctx, &binding)
	}); err != nil {
		return ComponentFilterAddResult{}, err
	}

	return ComponentFilterAddResult{Chat: *chat, Source: *source, SourceBinding: *sourceBinding, Filter: *filter, Binding: binding}, nil
}

func (s *service) RemoveChatComponentFilter(ctx context.Context, chatRef string, sourceRef string, externalChannelID string, filterRef string) (ComponentFilterRemoveResult, error) {
	if s == nil || s.Storage == nil {
		return ComponentFilterRemoveResult{}, fmt.Errorf("missing app storage")
	}
	chat, source, sourceBinding, err := s.resolveChatSourceBinding(ctx, chatRef, sourceRef, externalChannelID)
	if err != nil {
		return ComponentFilterRemoveResult{}, err
	}
	filter, err := s.resolveInboundEventFilterRegistration(ctx, filterRef)
	if err != nil {
		return ComponentFilterRemoveResult{}, err
	}

	disabled := false
	if err := s.Storage.Transaction(ctx, func(tx repository.Storage) error {
		current, err := tx.InboundFilterBindings().GetBySourceBindingAndFilter(ctx, sourceBinding.ID, filter.ID)
		if err != nil {
			return err
		}
		if current == nil || !current.Enabled {
			return nil
		}
		current.Enabled = false
		disabled = true
		return tx.InboundFilterBindings().Save(ctx, current)
	}); err != nil {
		return ComponentFilterRemoveResult{}, err
	}

	return ComponentFilterRemoveResult{Chat: *chat, Source: *source, SourceBinding: *sourceBinding, Filter: *filter, Disabled: disabled}, nil
}

func (s *service) ClearChatComponentFilters(ctx context.Context, chatRef string, sourceRef string, externalChannelID string) (ComponentFilterClearResult, error) {
	if s == nil || s.Storage == nil {
		return ComponentFilterClearResult{}, fmt.Errorf("missing app storage")
	}
	chat, source, sourceBinding, err := s.resolveChatSourceBinding(ctx, chatRef, sourceRef, externalChannelID)
	if err != nil {
		return ComponentFilterClearResult{}, err
	}

	disabled := 0
	if err := s.Storage.Transaction(ctx, func(tx repository.Storage) error {
		existing, err := tx.InboundFilterBindings().ListEnabledBySourceBindingID(ctx, sourceBinding.ID)
		if err != nil {
			return err
		}
		for _, binding := range existing {
			binding.Enabled = false
			if err := tx.InboundFilterBindings().Save(ctx, &binding); err != nil {
				return err
			}
			disabled++
		}
		return nil
	}); err != nil {
		return ComponentFilterClearResult{}, err
	}

	return ComponentFilterClearResult{Chat: *chat, Source: *source, SourceBinding: *sourceBinding, Disabled: disabled}, nil
}

func (s *service) ListChatComponentFilters(ctx context.Context, chatRef string, sourceRef string, externalChannelID string) (ComponentFilterListResult, error) {
	if s == nil || s.Storage == nil {
		return ComponentFilterListResult{}, fmt.Errorf("missing app storage")
	}
	chat, source, sourceBinding, err := s.resolveChatSourceBinding(ctx, chatRef, sourceRef, externalChannelID)
	if err != nil {
		return ComponentFilterListResult{}, err
	}
	bindings, err := s.Storage.InboundFilterBindings().ListEnabledBySourceBindingID(ctx, sourceBinding.ID)
	if err != nil {
		return ComponentFilterListResult{}, err
	}
	result := ComponentFilterListResult{
		Chat:          *chat,
		Source:        *source,
		SourceBinding: *sourceBinding,
		Bindings:      make([]ComponentFilterListBinding, 0, len(bindings)),
	}
	for _, binding := range bindings {
		filterRef := binding.FilterComponentID.String()
		registration, err := s.Storage.Components().GetByID(ctx, binding.FilterComponentID)
		if err != nil {
			return ComponentFilterListResult{}, err
		}
		if registration != nil {
			filterRef = registration.Ref()
		}
		result.Bindings = append(result.Bindings, ComponentFilterListBinding{
			Binding:   binding,
			FilterRef: filterRef,
		})
	}
	return result, nil
}

func (s *service) resolveChatSourceBinding(ctx context.Context, chatRef string, sourceRef string, externalChannelID string) (*coremodel.Chat, *coremodel.Component, *coremodel.ChatComponent, error) {
	chatID, err := s.ResolveChatRef(ctx, chatRef)
	if err != nil {
		return nil, nil, nil, err
	}
	chat, err := s.Storage.Chats().GetByID(ctx, chatID)
	if err != nil {
		return nil, nil, nil, err
	}
	if chat == nil {
		return nil, nil, nil, fmt.Errorf("chat not found: %s", strings.TrimSpace(chatRef))
	}
	source, err := s.resolveInboundSourceRegistration(ctx, sourceRef)
	if err != nil {
		return nil, nil, nil, err
	}
	binding, err := s.findEnabledSourceBinding(ctx, chat.ID, source.ID, externalChannelID)
	if err != nil {
		return nil, nil, nil, err
	}
	return chat, source, binding, nil
}

func (s *service) resolveInboundSourceRegistration(ctx context.Context, ref string) (*coremodel.Component, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("missing source component ref")
	}
	registration, err := s.resolveComponentRegistration(ctx, ref)
	if err != nil {
		return nil, err
	}
	loaded, err := s.resolveLoadedComponent(ctx, registration.ID)
	if err != nil {
		return nil, err
	}
	if _, ok := loaded.Component.(component.InboundSource); !ok {
		return nil, fmt.Errorf("component %s does not support inbound source", registration.Ref())
	}
	return registration, nil
}

func (s *service) findEnabledSourceBinding(ctx context.Context, chatID modeluuid.UUID, sourceComponentID modeluuid.UUID, externalChannelID string) (*coremodel.ChatComponent, error) {
	externalChannelID = strings.TrimSpace(externalChannelID)
	bindings, err := s.Storage.ChatComponents().ListEnabledByChatID(ctx, chatID)
	if err != nil {
		return nil, err
	}
	var matches []coremodel.ChatComponent
	for _, binding := range bindings {
		if binding.Role != coremodel.ChatComponentRoleSource || binding.ComponentID != sourceComponentID {
			continue
		}
		if externalChannelID != "" && strings.TrimSpace(binding.ExternalChannelID) != externalChannelID {
			continue
		}
		matches = append(matches, binding)
	}
	switch len(matches) {
	case 0:
		if externalChannelID != "" {
			return nil, fmt.Errorf("source binding not found for external_channel_id %q", externalChannelID)
		}
		return nil, fmt.Errorf("source binding not found for component in chat")
	case 1:
		return &matches[0], nil
	default:
		return nil, ambiguousSourceBindingError(matches)
	}
}

func ambiguousSourceBindingError(bindings []coremodel.ChatComponent) error {
	var channels []string
	for _, binding := range bindings {
		channels = append(channels, strings.TrimSpace(binding.ExternalChannelID))
	}
	return fmt.Errorf("multiple source bindings match; specify --external-channel-id: %s", strings.Join(channels, ", "))
}

func (s *service) resolveInboundEventFilterRegistration(ctx context.Context, ref string) (*coremodel.Component, error) {
	registration, err := s.resolveComponentRegistration(ctx, ref)
	if err != nil {
		return nil, err
	}
	loaded, err := s.resolveLoadedComponent(ctx, registration.ID)
	if err != nil {
		return nil, err
	}
	if _, ok := loaded.Component.(interface {
		component.Component
		inbound.Filterer
	}); !ok {
		return nil, fmt.Errorf("component %s does not support inbound event filtering", registration.Ref())
	}
	return registration, nil
}
