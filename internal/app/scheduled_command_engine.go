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
	var loadedComponents []*component.Loaded
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
		loadedComponents = append(loadedComponents, loaded)
	}

	updateFeeds := scheduledUpdateFeeds(loadedComponents)
	for _, loaded := range loadedComponents {
		if receiver, ok := loaded.Component.(component.UpdateFeedReceiver); ok {
			receiver.SetUpdateFeeds(updateFeeds)
		}
	}

	var bound []commandset.BoundSurface
	for _, loaded := range loadedComponents {
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

func (s *service) UpdateFeeds(ctx context.Context) ([]component.UpdateFeed, error) {
	if s == nil || s.Storage == nil {
		return nil, nil
	}
	registrations, err := s.Storage.Components().ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	var loadedComponents []*component.Loaded
	for _, registration := range registrations {
		loaded, err := s.ResolveComponent(ctx, registration.ID)
		if err != nil {
			return nil, err
		}
		if loaded != nil {
			loadedComponents = append(loadedComponents, loaded)
		}
	}
	return scheduledUpdateFeeds(loadedComponents), nil
}

func scheduledUpdateFeeds(loaded []*component.Loaded) []component.UpdateFeed {
	var feeds []component.UpdateFeed
	for _, item := range loaded {
		if item == nil || item.Component == nil {
			continue
		}
		if feed, ok := item.Component.(component.UpdateFeed); ok {
			feeds = append(feeds, feed)
		}
	}
	return feeds
}
