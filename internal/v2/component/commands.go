package component

import (
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

// CommandEngineForBindings builds a command engine for the command surfaces
// enabled by the provided chat component bindings.
//
// Preliminary location: the registry owns command-surface assembly for now
// because both broker message commands and future hostbridge commands need the
// same component-selection behavior.
func (r *Registry) CommandEngineForBindings(bindings []coremodel.ChatComponent, source commandengine.Source) (*commandengine.Engine, error) {
	return buildCommandEngine(r.CommandSurfacesForBindings(bindings), source)
}

func (r *Registry) CommandSurfacesForBindings(bindings []coremodel.ChatComponent) []CommandSurface {
	if r == nil {
		return nil
	}
	var out []CommandSurface
	for _, surface := range r.CommandSurfaces() {
		if matchesAnyBinding(surface, bindings) {
			out = append(out, surface)
		}
	}
	return out
}

func matchesAnyBinding(component Component, bindings []coremodel.ChatComponent) bool {
	for _, binding := range bindings {
		if MatchesBinding(component, binding) {
			return true
		}
	}
	return false
}

func buildCommandEngine(surfaces []CommandSurface, source commandengine.Source) (*commandengine.Engine, error) {
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
