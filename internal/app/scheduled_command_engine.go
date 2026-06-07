package app

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/component"
)

func (s *service) ScheduledCommandEngine(ctx context.Context) (*commandengine.Engine, error) {
	if s == nil || s.Storage == nil {
		return nil, nil
	}
	registrations, err := s.Storage.Components().ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	var bound []commandset.BoundSurface
	for _, registration := range registrations {
		loaded, err := s.ResolveComponent(ctx, registration.ID)
		if err != nil {
			return nil, err
		}
		if loaded == nil {
			continue
		}
		if receiver, ok := loaded.Component.(component.SearchMessageSourceReceiver); ok {
			receiver.SetSearchMessageSource(s)
		}
		surface, ok := loaded.Component.(component.CommandSurface)
		if !ok {
			continue
		}
		bound = append(bound, commandset.BoundSurface{
			Surface:       surface,
			ComponentRef:  loaded.Registration.Ref(),
			ComponentType: loaded.Registration.Type,
		})
	}
	return commandset.NewBoundEngineForSource(commandengine.SourceScheduler, bound)
}
