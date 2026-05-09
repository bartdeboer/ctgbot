package system

import (
	"context"

	brokerpkg "github.com/bartdeboer/ctgbot/internal/broker"
	"github.com/bartdeboer/ctgbot/internal/component"
	brokercomponent "github.com/bartdeboer/ctgbot/internal/component/broker"
	configcomponent "github.com/bartdeboer/ctgbot/internal/component/config"
	messagingcomponent "github.com/bartdeboer/ctgbot/internal/component/messaging"
	"github.com/bartdeboer/ctgbot/internal/messaging"
)

func (s *System) ResolveGlobalCommandSurfaces(ctx context.Context, b *brokerpkg.Broker) ([]component.CommandSurface, error) {
	_ = ctx
	if s == nil {
		return []component.CommandSurface{brokercomponent.New(b)}, nil
	}

	surfaces := []component.CommandSurface{
		brokercomponent.New(b),
		messagingcomponent.New(messaging.New(s.Storage, b)),
	}
	if s.Config != nil {
		configSurface, err := configcomponent.New(s.Config)
		if err != nil {
			return nil, err
		}
		if configSurface != nil {
			surfaces = append(surfaces, configSurface)
		}
	}
	return surfaces, nil
}
