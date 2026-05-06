package commandset

import (
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
)

func DefinitionsForSource(source commandengine.Source, surfaces ...component.CommandSurface) []commandengine.Definition {
	var definitions []commandengine.Definition
	for _, surface := range surfaces {
		if surface == nil {
			continue
		}
		definitions = append(definitions, surface.CommandDefinitions()...)
	}
	return filterDefinitionsBySource(definitions, source)
}

func NewRouterForSource(source commandengine.Source, surfaces ...component.CommandSurface) (*commandengine.Router, error) {
	definitions := DefinitionsForSource(source, surfaces...)
	if len(definitions) == 0 {
		return nil, fmt.Errorf("no command definitions for source %s", source)
	}
	return commandengine.NewRouter(definitions, source)
}

func NewEngineForSource(source commandengine.Source, surfaces ...component.CommandSurface) (*commandengine.Engine, error) {
	if len(surfaces) == 0 {
		return nil, nil
	}
	router, err := NewRouterForSource(source, surfaces...)
	if err != nil {
		return nil, err
	}
	registry := commandengine.NewRegistry()
	for _, surface := range surfaces {
		if surface == nil {
			continue
		}
		if err := surface.RegisterCommandHandlers(registry); err != nil {
			return nil, err
		}
	}
	return commandengine.NewEngine(router, registry), nil
}

func filterDefinitionsBySource(definitions []commandengine.Definition, source commandengine.Source) []commandengine.Definition {
	out := make([]commandengine.Definition, 0, len(definitions))
	for _, definition := range definitions {
		if definition.AllowsSource(source) {
			out = append(out, definition)
		}
	}
	return out
}

func CanonicalRoutePatterns(definitions []commandengine.Definition, actor simplerbac.Actor) []string {
	out := make([]string, 0, len(definitions))
	seen := map[string]struct{}{}
	for _, definition := range definitions {
		if err := definition.Policy.Check(actor); err != nil {
			continue
		}
		if len(definition.Routes) == 0 {
			continue
		}
		pattern := commandengine.NormalizePattern(definition.Routes[0].Pattern)
		if strings.TrimSpace(pattern) == "" {
			continue
		}
		if _, ok := seen[pattern]; ok {
			continue
		}
		seen[pattern] = struct{}{}
		out = append(out, pattern)
	}
	return out
}
