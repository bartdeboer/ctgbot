package app

import (
	"context"

	broker "github.com/bartdeboer/ctgbot/internal/broker"
	"github.com/bartdeboer/ctgbot/internal/component"
	componentadmin "github.com/bartdeboer/ctgbot/internal/component/admin"
	brokercomponent "github.com/bartdeboer/ctgbot/internal/component/broker"
	configcomponent "github.com/bartdeboer/ctgbot/internal/component/config"
	allowlistfilter "github.com/bartdeboer/ctgbot/internal/component/filter/allowlist"
	messagingcomponent "github.com/bartdeboer/ctgbot/internal/component/messaging"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/messaging"
)

func (s *service) CommandSurfaces(ctx context.Context, chat coremodel.Chat, deps broker.CommandSurfaceDeps) ([]component.CommandSurface, error) {
	surfaces := []component.CommandSurface{
		componentadmin.New(s.Repository(), s),
		messagingcomponent.New(messaging.New(s.Repository()), deps.Inbound),
	}
	if deps.BrokerActions != nil {
		surfaces = append(surfaces, brokercomponent.New(deps.BrokerActions))
	}
	if hasAllowlist, err := s.chatHasAllowlistFilter(ctx, chat); err != nil {
		return nil, err
	} else if hasAllowlist {
		surfaces = append(surfaces, allowlistfilter.New(s.Repository()))
	}
	if config := s.AppConfig(); config != nil {
		configSurface, err := configcomponent.New(config)
		if err != nil {
			return nil, err
		}
		if configSurface != nil {
			surfaces = append(surfaces, configSurface)
		}
	}
	return surfaces, nil
}

func (s *service) chatHasAllowlistFilter(ctx context.Context, chat coremodel.Chat) (bool, error) {
	if s == nil || s.Storage == nil || chat.ID.IsNull() {
		return false, nil
	}
	bindings, err := s.Storage.ChatComponents().ListEnabledByChatID(ctx, chat.ID)
	if err != nil {
		return false, err
	}
	for _, sourceBinding := range bindings {
		if sourceBinding.Role != coremodel.ChatComponentRoleSource {
			continue
		}
		filterBindings, err := s.Storage.InboundFilterBindings().ListEnabledBySourceBindingID(ctx, sourceBinding.ID)
		if err != nil {
			return false, err
		}
		for _, filterBinding := range filterBindings {
			registration, err := s.Storage.Components().GetByID(ctx, filterBinding.FilterComponentID)
			if err != nil {
				return false, err
			}
			if registration != nil && registration.Enabled && allowlistfilter.IsRegistration(registration) {
				return true, nil
			}
		}
	}
	return false, nil
}
