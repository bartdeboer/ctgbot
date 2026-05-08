package commandset

import (
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

type BoundSurface struct {
	Surface       component.CommandSurface
	ComponentRef  string
	ComponentType string
}

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

func DefinitionsForBoundSource(source commandengine.Source, bound []BoundSurface, globals ...component.CommandSurface) []commandengine.Definition {
	definitions, _ := buildBoundDefinitions(source, bound, globals...)
	return definitions
}

func NewRouterForSource(source commandengine.Source, surfaces ...component.CommandSurface) (*commandengine.Router, error) {
	definitions := DefinitionsForSource(source, surfaces...)
	if len(definitions) == 0 {
		return nil, fmt.Errorf("no command definitions for source %s", source)
	}
	return commandengine.NewRouter(definitions, source)
}

func NewBoundRouterForSource(source commandengine.Source, bound []BoundSurface, globals ...component.CommandSurface) (*commandengine.Router, error) {
	definitions := DefinitionsForBoundSource(source, bound, globals...)
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

func NewBoundEngineForSource(source commandengine.Source, bound []BoundSurface, globals ...component.CommandSurface) (*commandengine.Engine, error) {
	if len(bound) == 0 && len(globals) == 0 {
		return nil, nil
	}
	definitions, registrars := buildBoundDefinitions(source, bound, globals...)
	if len(definitions) == 0 {
		return nil, fmt.Errorf("no command definitions for source %s", source)
	}
	router, err := commandengine.NewRouter(definitions, source)
	if err != nil {
		return nil, err
	}
	registry := commandengine.NewRegistry()
	for _, register := range registrars {
		if err := register(registry); err != nil {
			return nil, err
		}
	}
	return commandengine.NewEngine(router, registry), nil
}

type handlerRegistrar func(registry *commandengine.Registry) error

func buildBoundDefinitions(source commandengine.Source, bound []BoundSurface, globals ...component.CommandSurface) ([]commandengine.Definition, []handlerRegistrar) {
	var (
		definitions []commandengine.Definition
		registrars  []handlerRegistrar
	)

	for _, surface := range globals {
		if surface == nil {
			continue
		}
		definitionsForSource := DefinitionsForSource(source, surface)
		if len(definitionsForSource) == 0 {
			continue
		}
		definitions = append(definitions, definitionsForSource...)
		surface := surface
		registrars = append(registrars, func(registry *commandengine.Registry) error {
			return surface.RegisterCommandHandlers(registry)
		})
	}

	typeCounts := map[string]int{}
	for _, binding := range bound {
		if !isLocalCommandSurface(binding.Surface) {
			continue
		}
		definitionsForSource := DefinitionsForSource(source, binding.Surface)
		if len(definitionsForSource) == 0 {
			continue
		}
		componentType := strings.TrimSpace(binding.ComponentType)
		if componentType == "" {
			continue
		}
		typeCounts[componentType]++
	}

	for _, binding := range bound {
		if binding.Surface == nil {
			continue
		}
		if !isLocalCommandSurface(binding.Surface) {
			definitionsForSource := DefinitionsForSource(source, binding.Surface)
			if len(definitionsForSource) == 0 {
				continue
			}
			definitions = append(definitions, definitionsForSource...)
			surface := binding.Surface
			registrars = append(registrars, func(registry *commandengine.Registry) error {
				return surface.RegisterCommandHandlers(registry)
			})
			continue
		}
		wrapped, definitionPrefix := wrapLocalDefinitions(source, binding, typeCounts)
		if len(wrapped) == 0 {
			continue
		}
		definitions = append(definitions, wrapped...)
		surface := binding.Surface
		prefix := definitionPrefix
		registrars = append(registrars, func(registry *commandengine.Registry) error {
			return surface.RegisterCommandHandlers(registry.WithPatternPrefix(prefix))
		})
	}

	return definitions, registrars
}

func isLocalCommandSurface(surface component.CommandSurface) bool {
	if surface == nil {
		return false
	}
	local, ok := surface.(component.LocalCommandSurface)
	return ok && local.UsesLocalCommandRoutes()
}

func wrapLocalDefinitions(source commandengine.Source, binding BoundSurface, typeCounts map[string]int) ([]commandengine.Definition, string) {
	localDefinitions := DefinitionsForSource(source, binding.Surface)
	fullPrefix := strings.TrimSpace(binding.ComponentRef)
	visiblePrefix := fullPrefix
	if typeCounts[strings.TrimSpace(binding.ComponentType)] <= 1 {
		if componentType := strings.TrimSpace(binding.ComponentType); componentType != "" {
			visiblePrefix = componentType
		}
	}
	out := make([]commandengine.Definition, 0, len(localDefinitions))
	for _, definition := range localDefinitions {
		localPattern := commandengine.NormalizePattern(definition.Pattern)
		if localPattern == "" {
			continue
		}
		out = append(out, namespaceDefinition(definition, visiblePrefix, fullPrefix))
	}
	return out, fullPrefix
}

func namespaceDefinition(definition commandengine.Definition, visiblePrefix string, fullPrefix string) commandengine.Definition {
	wrapped := definition
	wrapped.Pattern = prefixedPattern(fullPrefix, definition.Pattern)
	wrapped.Aliases = nil
	if strings.TrimSpace(visiblePrefix) != strings.TrimSpace(fullPrefix) && !definition.Absolute {
		wrapped.Hidden = true
		wrapped.Aliases = append(wrapped.Aliases, commandengine.Route{
			Pattern: prefixedPattern(visiblePrefix, definition.Pattern),
		})
	}
	wrapped.Aliases = append(wrapped.Aliases, namespaceAliases(definition.Aliases, visiblePrefix, fullPrefix)...)
	return wrapped
}

func namespaceAliases(aliases []commandengine.Route, visiblePrefix string, fullPrefix string) []commandengine.Route {
	out := make([]commandengine.Route, 0, len(aliases)*2)
	for _, alias := range aliases {
		if alias.Absolute {
			out = append(out, alias)
			continue
		}
		fullAlias := alias
		fullAlias.Pattern = prefixedPattern(fullPrefix, alias.Pattern)
		if strings.TrimSpace(visiblePrefix) != strings.TrimSpace(fullPrefix) {
			fullAlias.Hidden = true
		}
		out = append(out, fullAlias)
		if strings.TrimSpace(visiblePrefix) == strings.TrimSpace(fullPrefix) {
			continue
		}
		visibleAlias := alias
		visibleAlias.Pattern = prefixedPattern(visiblePrefix, alias.Pattern)
		out = append(out, visibleAlias)
	}
	return out
}

func prefixedPattern(prefix string, pattern string) string {
	return commandengine.JoinPattern(prefix, pattern)
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
		pattern := ""
		for _, route := range definition.Routes() {
			if route.Hidden {
				continue
			}
			pattern = commandengine.NormalizePattern(route.Pattern)
			break
		}
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
