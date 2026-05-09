package cmdsurface

import (
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandset"
	componentpkg "github.com/bartdeboer/ctgbot/internal/component"
	componentadmin "github.com/bartdeboer/ctgbot/internal/component/admin"
	brokercomponent "github.com/bartdeboer/ctgbot/internal/component/broker"
	codexcomponent "github.com/bartdeboer/ctgbot/internal/component/codex"
	configcomponent "github.com/bartdeboer/ctgbot/internal/component/config"
	llamacppcomponent "github.com/bartdeboer/ctgbot/internal/component/llamacpp"
	messagingcomponent "github.com/bartdeboer/ctgbot/internal/component/messaging"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
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

func GlobalSurfaces() []componentpkg.CommandSurface {
	return []componentpkg.CommandSurface{
		componentadmin.New(nil, nil),
		brokercomponent.New(nil),
		messagingcomponent.New(nil, nil),
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

func RegisterGobTypes(register func(any)) {
	componentadmin.RegisterGobTypes(register)
	codexcomponent.RegisterGobTypes(register)
	llamacppcomponent.RegisterGobTypes(register)
	messagingcomponent.RegisterGobTypes(register)
}

func GlobalDirectPrefixes() []string {
	return []string{"component", "thread"}
}

func surfaceForType(componentType string) (componentpkg.CommandSurface, bool) {
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
