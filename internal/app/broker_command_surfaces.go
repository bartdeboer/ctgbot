package app

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/brokercontract"
	"github.com/bartdeboer/ctgbot/internal/component"
	componentadmin "github.com/bartdeboer/ctgbot/internal/component/admin"
	brokercomponent "github.com/bartdeboer/ctgbot/internal/component/broker"
	configcomponent "github.com/bartdeboer/ctgbot/internal/component/config"
	allowlistfilter "github.com/bartdeboer/ctgbot/internal/component/filter/allowlist"
	messagingcomponent "github.com/bartdeboer/ctgbot/internal/component/messaging"
	"github.com/bartdeboer/ctgbot/internal/messaging"
)

func (s *service) CommandSurfaces(ctx context.Context, deps brokercontract.CommandSurfaceDeps) ([]component.CommandSurface, error) {
	_ = ctx
	surfaces := []component.CommandSurface{
		componentadmin.New(s.Repository(), s),
		allowlistfilter.New(s.Repository()),
		messagingcomponent.New(messaging.New(s.Repository()), deps.Inbound),
	}
	if deps.BrokerActions != nil {
		surfaces = append(surfaces, brokercomponent.New(deps.BrokerActions))
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
