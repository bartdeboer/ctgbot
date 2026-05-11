package coremodel

import (
	"fmt"
	"strings"
)

type ParsedComponentRef struct {
	Type         string
	Name         string
	ExplicitName bool
}

func ParseComponentRef(ref string) (ParsedComponentRef, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ParsedComponentRef{}, fmt.Errorf("missing component reference")
	}
	parts := strings.Split(ref, "/")
	switch len(parts) {
	case 1:
		componentType := strings.TrimSpace(parts[0])
		if componentType == "" {
			return ParsedComponentRef{}, fmt.Errorf("missing component type")
		}
		return ParsedComponentRef{
			Type: componentType,
			Name: DefaultComponentName(componentType),
		}, nil
	case 2:
		componentType := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		if componentType == "" {
			return ParsedComponentRef{}, fmt.Errorf("missing component type")
		}
		if name == "" {
			return ParsedComponentRef{}, fmt.Errorf("missing component name")
		}
		return ParsedComponentRef{
			Type:         componentType,
			Name:         name,
			ExplicitName: true,
		}, nil
	default:
		return ParsedComponentRef{}, fmt.Errorf("invalid component reference: %q", ref)
	}
}

func (r ParsedComponentRef) ResolvedName() string {
	if name := strings.TrimSpace(r.Name); name != "" {
		return name
	}
	return DefaultComponentName(r.Type)
}

func (r ParsedComponentRef) Ref() string {
	return ComponentRef(r.Type, r.ResolvedName())
}

func (r ChatComponentRole) Valid() bool {
	switch r {
	case ChatComponentRoleSource, ChatComponentRoleRelay, ChatComponentRoleAgent, ChatComponentRoleCommand:
		return true
	default:
		return false
	}
}

func (r ComponentBindingRole) Valid() bool {
	switch r {
	case ComponentBindingRoleGuard:
		return true
	default:
		return false
	}
}
