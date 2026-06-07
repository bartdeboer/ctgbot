package cmdsurface

import (
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandset"
	componentpkg "github.com/bartdeboer/ctgbot/internal/component"
	componentadmin "github.com/bartdeboer/ctgbot/internal/component/admin"
	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
	brokercomponent "github.com/bartdeboer/ctgbot/internal/component/broker"
	claudecomponent "github.com/bartdeboer/ctgbot/internal/component/claude"
	codexcomponent "github.com/bartdeboer/ctgbot/internal/component/codex"
	configcomponent "github.com/bartdeboer/ctgbot/internal/component/config"
	gmailcomponent "github.com/bartdeboer/ctgbot/internal/component/gmail"
	gmailv2component "github.com/bartdeboer/ctgbot/internal/component/gmailv2"
	heartbeatcomponent "github.com/bartdeboer/ctgbot/internal/component/heartbeat"
	indexingcomponent "github.com/bartdeboer/ctgbot/internal/component/indexing"
	llamacppcomponent "github.com/bartdeboer/ctgbot/internal/component/llamacpp"
	llamacppagentcomponent "github.com/bartdeboer/ctgbot/internal/component/llamacppagent"
	messagingcomponent "github.com/bartdeboer/ctgbot/internal/component/messaging"
	modelcomponent "github.com/bartdeboer/ctgbot/internal/component/model"
	opscomponent "github.com/bartdeboer/ctgbot/internal/component/ops"
	remotecomponent "github.com/bartdeboer/ctgbot/internal/component/remote"
	schedulercomponent "github.com/bartdeboer/ctgbot/internal/component/scheduler"
	semanticcomponent "github.com/bartdeboer/ctgbot/internal/component/semantic"
	sqlcomponent "github.com/bartdeboer/ctgbot/internal/component/sql"
	supertoniccomponent "github.com/bartdeboer/ctgbot/internal/component/supertonic"
	whispercppcomponent "github.com/bartdeboer/ctgbot/internal/component/whispercpp"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

type ResolvedSurfaceSet struct {
	ComponentRef  string
	ComponentType string
	Surface       componentpkg.CommandSurface
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
	surface, supported := surfaceForType(parsed.Type)
	return ResolvedSurfaceSet{
		ComponentRef:  parsed.Ref(),
		ComponentType: parsed.Type,
		Surface:       surface,
		Supported:     supported,
	}
}

func GlobalSurfaces() []componentpkg.CommandSurface {
	return []componentpkg.CommandSurface{
		componentadmin.New(nil, nil),
		brokercomponent.New(nil),
		messagingcomponent.New(nil, nil),
		remotecomponent.New(nil),
		(*configcomponent.Component)(nil),
	}
}

// ParseOnlySurfaces are known hostbridge client-side command shapes whose
// server-side availability is still decided by the bound chat runtime. They let
// the hostbridge binary construct typed command DTOs without making the command
// globally executable.
func ParseOnlySurfaces() []componentpkg.CommandSurface {
	return []componentpkg.CommandSurface{
		(*sqlcomponent.Component)(nil),
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
	configsurface.RegisterGobTypes(register)
	componentadmin.RegisterGobTypes(register)
	brokercomponent.RegisterGobTypes(register)
	sqlcomponent.RegisterGobTypes(register)
	gmailcomponent.RegisterGobTypes(register)
	gmailv2component.RegisterGobTypes(register)
	heartbeatcomponent.RegisterGobTypes(register)
	indexingcomponent.RegisterGobTypes(register)
	schedulercomponent.RegisterGobTypes(register)
	semanticcomponent.RegisterGobTypes(register)
	agentcommon.RegisterGobTypes(register)
	llamacppcomponent.RegisterGobTypes(register)
	llamacppagentcomponent.RegisterGobTypes(register)
	modelcomponent.RegisterGobTypes(register)
	opscomponent.RegisterGobTypes(register)
	remotecomponent.RegisterGobTypes(register)
	messagingcomponent.RegisterGobTypes(register)
	supertoniccomponent.RegisterGobTypes(register)
	whispercppcomponent.RegisterGobTypes(register)
}

func GlobalDirectPrefixes() []string {
	return []string{"component", "status", "thread", "turn", "model", "sql", "remote"}
}

func surfaceForType(componentType string) (componentpkg.CommandSurface, bool) {
	switch strings.TrimSpace(componentType) {
	case claudecomponent.Type:
		return (*claudecomponent.Component)(nil), true
	case codexcomponent.Type:
		return (*codexcomponent.Component)(nil), true
	case gmailcomponent.Type:
		return (*gmailcomponent.Component)(nil), true
	case gmailv2component.Type:
		return (*gmailv2component.Component)(nil), true
	case heartbeatcomponent.Type:
		return (*heartbeatcomponent.Component)(nil), true
	case indexingcomponent.Type:
		return (*indexingcomponent.Component)(nil), true
	case indexingcomponent.SearchType:
		return (*indexingcomponent.SearchComponent)(nil), true
	case llamacppcomponent.Type:
		return (*llamacppcomponent.Component)(nil), true
	case llamacppagentcomponent.Type:
		return (*llamacppagentcomponent.Component)(nil), true
	case modelcomponent.Type:
		return (*modelcomponent.Component)(nil), true
	case opscomponent.Type:
		return (*opscomponent.Component)(nil), true
	case schedulercomponent.Type:
		return (*schedulercomponent.Component)(nil), true
	case semanticcomponent.Type:
		return (*semanticcomponent.Component)(nil), true
	case supertoniccomponent.Type:
		return (*supertoniccomponent.Component)(nil), true
	case whispercppcomponent.Type:
		return (*whispercppcomponent.Component)(nil), true
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

func CommandRefBoundSurfaces(ref string) []commandset.BoundSurface {
	parsed, err := coremodel.ParseComponentRef(strings.TrimSpace(ref))
	if err != nil {
		return nil
	}
	componentRef := parsed.Ref()
	var out []commandset.BoundSurface
	if surface, ok := surfaceForType(parsed.Type); ok {
		out = append(out, commandset.BoundSurface{
			Surface:       surface,
			ComponentRef:  componentRef,
			ComponentType: parsed.Type,
		})
	}
	return out
}
