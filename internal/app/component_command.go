package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

type ComponentCommandRequest struct {
	ComponentRef string
	RuntimeKind  string
	HomePath     string
	Args         []string
}

type ComponentCommandResult struct {
	Text string
}

func (s *service) RunComponentCommand(ctx context.Context, request ComponentCommandRequest) (ComponentCommandResult, error) {
	var registration *coremodel.Component
	var err error
	if strings.TrimSpace(request.RuntimeKind) != "" || strings.TrimSpace(request.HomePath) != "" {
		manager, err := s.componentManager()
		if err != nil {
			return ComponentCommandResult{}, err
		}
		registration, err = manager.EnsureComponent(ctx, strings.TrimSpace(request.ComponentRef), strings.TrimSpace(request.RuntimeKind), strings.TrimSpace(request.HomePath))
	} else {
		registration, err = s.resolveComponentRegistration(ctx, request.ComponentRef)
	}
	if err != nil {
		return ComponentCommandResult{}, err
	}
	if registration == nil {
		return ComponentCommandResult{}, fmt.Errorf("component registration returned nil")
	}

	loaded, err := s.resolveLoadedComponent(ctx, registration.ID)
	if err != nil {
		return ComponentCommandResult{}, err
	}
	bound := boundCLIComponentSurfaces(loaded)
	if len(bound) == 0 {
		return ComponentCommandResult{}, fmt.Errorf("component has no CLI commands: %s", registration.Ref())
	}
	definitions := commandset.DefinitionsForBoundSource(commandengine.SourceCLI, bound)
	if len(request.Args) == 0 {
		return ComponentCommandResult{Text: componentCLIHelpText(definitions)}, nil
	}
	engine, err := commandset.NewBoundEngineForSource(commandengine.SourceCLI, bound)
	if err != nil {
		return ComponentCommandResult{}, err
	}
	base := commandengine.Request{
		Context: commandengine.Context{
			Source: commandengine.SourceCLI,
			Actor: commandengine.Actor{
				ID:    "cli",
				Roles: []simplerbac.Role{simplerbac.RoleRoot},
			},
		},
	}
	result, err := engine.Run(ctx, base, append([]string{registration.Ref()}, request.Args...))
	if err != nil {
		return ComponentCommandResult{}, err
	}
	return ComponentCommandResult{Text: result.Text}, nil
}

func boundCLIComponentSurfaces(loaded *component.Loaded) []commandset.BoundSurface {
	if loaded == nil || loaded.Component == nil {
		return nil
	}
	componentRef := loaded.Registration.Ref()
	componentType := strings.TrimSpace(loaded.Registration.Type)
	var bound []commandset.BoundSurface
	if surface, ok := loaded.Component.(component.CommandSurface); ok {
		bound = append(bound, commandset.BoundSurface{
			Surface:       surface,
			ComponentRef:  componentRef,
			ComponentType: componentType,
		})
	}
	if surface := component.NewCLIAdminSurface(loaded.Component); surface != nil {
		bound = append(bound, commandset.BoundSurface{
			Surface:       surface,
			ComponentRef:  componentRef,
			ComponentType: componentType,
		})
	}
	return bound
}

func componentCLIHelpText(definitions []commandengine.Definition) string {
	patterns := commandset.CanonicalRoutePatterns(definitions, coremodel.Actor{
		Roles: []simplerbac.Role{simplerbac.RoleRoot},
	})
	if len(patterns) == 0 {
		return "no component CLI commands"
	}
	var builder strings.Builder
	builder.WriteString("available component commands:")
	for _, pattern := range patterns {
		builder.WriteString("\n  ")
		builder.WriteString(pattern)
	}
	return builder.String()
}
