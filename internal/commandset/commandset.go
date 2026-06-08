package commandset

import (
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
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

func DescriptionsForSource(source commandengine.Source, surfaces ...component.CommandSurface) []commandengine.Description {
	var descriptions []commandengine.Description
	for _, surface := range surfaces {
		describer, ok := surface.(component.CommandDescriptionSurface)
		if !ok || describer == nil {
			continue
		}
		descriptions = append(descriptions, describer.CommandDescriptions()...)
	}
	return filterDescriptionsBySource(descriptions, source)
}

func DefinitionsForBoundSource(source commandengine.Source, bound []BoundSurface, globals ...component.CommandSurface) []commandengine.Definition {
	definitions, _ := buildBoundDefinitions(source, bound, globals...)
	return definitions
}

func DescriptionsForBoundSource(source commandengine.Source, bound []BoundSurface, globals ...component.CommandSurface) []commandengine.Description {
	return buildBoundDescriptions(source, bound, globals...)
}

func NewRouterForSource(source commandengine.Source, surfaces ...component.CommandSurface) (*commandengine.Router, error) {
	definitions := DefinitionsForSource(source, surfaces...)
	if len(definitions) == 0 {
		return nil, fmt.Errorf("no command definitions for source %s", source)
	}
	return commandengine.NewRouter(definitions, source, DescriptionsForSource(source, surfaces...)...)
}

func NewBoundRouterForSource(source commandengine.Source, bound []BoundSurface, globals ...component.CommandSurface) (*commandengine.Router, error) {
	definitions := DefinitionsForBoundSource(source, bound, globals...)
	if len(definitions) == 0 {
		return nil, fmt.Errorf("no command definitions for source %s", source)
	}
	return commandengine.NewRouter(definitions, source, DescriptionsForBoundSource(source, bound, globals...)...)
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
	router, err := commandengine.NewRouter(definitions, source, buildBoundDescriptions(source, bound, globals...)...)
	if err != nil {
		return nil, err
	}
	registry := commandengine.NewRegistry()
	for _, register := range registrars {
		if err := register(registry); err != nil {
			return nil, err
		}
	}
	return commandengine.NewEngine(router, registry).WithActiveComponentRefs(activeComponentRefs(bound)), nil
}

func buildBoundDescriptions(source commandengine.Source, bound []BoundSurface, globals ...component.CommandSurface) []commandengine.Description {
	var descriptions []commandengine.Description
	for _, surface := range globals {
		descriptions = append(descriptions, DescriptionsForSource(source, surface)...)
	}
	typeCounts := map[string]int{}
	for _, binding := range bound {
		if !isLocalCommandSurface(binding.Surface) {
			continue
		}
		if len(DefinitionsForSource(source, binding.Surface)) == 0 {
			continue
		}
		componentType := strings.TrimSpace(binding.ComponentType)
		if componentType != "" {
			typeCounts[componentType]++
		}
	}
	for _, binding := range bound {
		desc := DescriptionsForSource(source, binding.Surface)
		if len(desc) == 0 {
			continue
		}
		if !isLocalCommandSurface(binding.Surface) {
			descriptions = append(descriptions, desc...)
			continue
		}
		componentType := strings.TrimSpace(binding.ComponentType)
		fullPrefix := strings.TrimSpace(binding.ComponentRef)
		visiblePrefix := fullPrefix
		if typeCounts[componentType] <= 1 && componentType != "" {
			visiblePrefix = componentType
		}
		for _, description := range desc {
			descriptions = append(descriptions, namespaceDescription(description, visiblePrefix, fullPrefix)...)
		}
	}
	return descriptions
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

func namespaceDescription(description commandengine.Description, visiblePrefix string, fullPrefix string) []commandengine.Description {
	localPattern := commandengine.NormalizePattern(description.Pattern)
	if description.Absolute {
		description.Pattern = localPattern
		return []commandengine.Description{description}
	}
	full := description
	full.Pattern = prefixedPattern(fullPrefix, localPattern)
	if strings.TrimSpace(visiblePrefix) == strings.TrimSpace(fullPrefix) {
		return []commandengine.Description{full}
	}
	full.Hidden = true
	visible := description
	visible.Pattern = prefixedPattern(visiblePrefix, localPattern)
	return []commandengine.Description{full, visible}
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

func filterDescriptionsBySource(descriptions []commandengine.Description, source commandengine.Source) []commandengine.Description {
	out := make([]commandengine.Description, 0, len(descriptions))
	for _, description := range descriptions {
		if description.AllowsSource(source) {
			out = append(out, description)
		}
	}
	return out
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

func CanonicalRoutePatterns(definitions []commandengine.Definition, actor coremodel.Actor) []string {
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

func InstructionRoutePatterns(definitions []commandengine.Definition, actor coremodel.Actor) []string {
	out := make([]string, 0, len(definitions))
	seen := map[string]struct{}{}
	add := func(pattern string) {
		pattern = commandengine.NormalizePattern(pattern)
		if pattern == "" {
			return
		}
		if _, ok := seen[pattern]; ok {
			return
		}
		seen[pattern] = struct{}{}
		out = append(out, pattern)
	}

	eligible := instructionEligibleDefinitions(definitions, actor)
	rootCounts := instructionRootCounts(eligible)
	for _, definition := range eligible {
		visibility := definition.InstructionVisibilityOrDefault()
		if visibility == commandengine.InstructionEssential || visibility == commandengine.InstructionImportant {
			for _, route := range definition.Routes() {
				if route.Hidden {
					continue
				}
				add(route.Pattern)
			}
		}
		for _, route := range definition.Routes() {
			if route.Hidden {
				continue
			}
			pattern := commandengine.NormalizePattern(route.Pattern)
			if isHelpRoutePattern(pattern) {
				add(pattern)
			}
			if root := instructionRoot(pattern); root != "" && rootCounts[root] > 1 {
				add(root + " help")
			}
		}
	}
	return out
}

func InstructionFamilyDescriptions(descriptions []commandengine.Description, actor coremodel.Actor) map[string]string {
	out := map[string]string{}
	for _, description := range descriptions {
		if description.Hidden {
			continue
		}
		if err := description.Policy.Check(actor); err != nil {
			continue
		}
		pattern := commandengine.NormalizePattern(description.Pattern)
		if pattern == "" || strings.Contains(pattern, " ") {
			continue
		}
		help := strings.TrimSpace(description.Help)
		if help == "" {
			continue
		}
		out[pattern] = help
	}
	return out
}

func instructionEligibleDefinitions(definitions []commandengine.Definition, actor coremodel.Actor) []commandengine.Definition {
	out := make([]commandengine.Definition, 0, len(definitions))
	for _, definition := range definitions {
		if err := definition.Policy.Check(actor); err != nil {
			continue
		}
		visibility := definition.InstructionVisibilityOrDefault()
		if visibility == commandengine.InstructionHidden {
			continue
		}
		out = append(out, definition)
	}
	return out
}

func instructionRootCounts(definitions []commandengine.Definition) map[string]int {
	counts := map[string]int{}
	for _, definition := range definitions {
		for _, route := range definition.Routes() {
			if route.Hidden {
				continue
			}
			if root := instructionRoot(route.Pattern); root != "" {
				counts[root]++
			}
		}
	}
	return counts
}

func isHelpRoutePattern(pattern string) bool {
	fields := strings.Fields(commandengine.NormalizePattern(pattern))
	return len(fields) > 0 && fields[len(fields)-1] == "help"
}

func instructionRoot(pattern string) string {
	fields := strings.Fields(commandengine.NormalizePattern(pattern))
	if len(fields) < 2 {
		return ""
	}
	return fields[0]
}

func activeComponentRefs(bound []BoundSurface) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(bound))
	for _, binding := range bound {
		ref := strings.TrimSpace(binding.ComponentRef)
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	return out
}
