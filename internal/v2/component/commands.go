package component

import (
	"github.com/bartdeboer/ctgbot/internal/commandengine"
)

func BuildCommandEngine(surfaces []CommandSurface, source commandengine.Source) (*commandengine.Engine, error) {
	var definitions []commandengine.Definition
	registry := commandengine.NewRegistry()
	for _, surface := range surfaces {
		if surface == nil {
			continue
		}
		definitions = append(definitions, surface.CommandDefinitions()...)
		if err := surface.RegisterCommandHandlers(registry); err != nil {
			return nil, err
		}
	}
	router, err := commandengine.NewRouter(definitions, source)
	if err != nil {
		return nil, err
	}
	return commandengine.NewEngine(router, registry), nil
}
