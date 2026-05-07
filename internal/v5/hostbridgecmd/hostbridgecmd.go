package hostbridgecmd

import (
	"strings"

	"github.com/bartdeboer/ctgbot/internal/v5/commandset"
	v5component "github.com/bartdeboer/ctgbot/internal/v5/component"
	brokercomponent "github.com/bartdeboer/ctgbot/internal/v5/component/broker"
	codexcomponent "github.com/bartdeboer/ctgbot/internal/v5/component/codex"
	configcomponent "github.com/bartdeboer/ctgbot/internal/v5/component/config"
	llamacppcomponent "github.com/bartdeboer/ctgbot/internal/v5/component/llamacpp"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
)

type ResolvedSurfaceSet struct {
	ComponentRef  string
	ComponentType string
	Supported     bool
}

const DefaultComponentType = codexcomponent.Type

func Resolve(ref string) ResolvedSurfaceSet {
	parsed, err := coremodel.ParseComponentRef(strings.TrimSpace(ref))
	if err != nil {
		parsed = coremodel.ParsedComponentRef{
			Type: DefaultComponentType,
			Name: coremodel.DefaultComponentName(DefaultComponentType),
		}
	}
	_, supported := surfaceForType(parsed.Type)
	return ResolvedSurfaceSet{
		ComponentRef:  parsed.Ref(),
		ComponentType: parsed.Type,
		Supported:     supported,
	}
}

func GlobalSurfaces() []v5component.CommandSurface {
	return []v5component.CommandSurface{
		brokercomponent.New(nil),
		(*configcomponent.Component)(nil),
	}
}

func BoundSurfaces(ref string) []commandset.BoundSurface {
	resolved := Resolve(ref)
	surface, ok := surfaceForType(resolved.ComponentType)
	if !ok {
		return nil
	}
	return []commandset.BoundSurface{{
		Surface:       surface,
		ComponentRef:  resolved.ComponentRef,
		ComponentType: resolved.ComponentType,
	}}
}

func DirectPrefixes(ref string) []string {
	resolved := Resolve(ref)
	prefixes := []string{resolved.ComponentType, resolved.ComponentRef}
	return dedupeNonEmpty(prefixes...)
}

func LegacyCodexShorthandEnabled(ref string) bool {
	return Resolve(ref).ComponentType == codexcomponent.Type
}

func surfaceForType(componentType string) (v5component.CommandSurface, bool) {
	switch strings.TrimSpace(componentType) {
	case codexcomponent.Type:
		return (*codexcomponent.Component)(nil), true
	case llamacppcomponent.Type:
		return (*llamacppcomponent.Component)(nil), true
	default:
		return nil, false
	}
}

func dedupeNonEmpty(values ...string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
