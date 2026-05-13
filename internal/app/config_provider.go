package app

import "github.com/bartdeboer/ctgbot/internal/appstate"

func (s *service) AppConfig() *appstate.Config {
	if s == nil || s.Resolver == nil {
		return nil
	}
	provider, ok := s.Resolver.(interface{ AppConfig() *appstate.Config })
	if !ok {
		return nil
	}
	return provider.AppConfig()
}
