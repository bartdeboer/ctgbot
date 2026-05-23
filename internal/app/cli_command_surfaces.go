package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	componentadmin "github.com/bartdeboer/ctgbot/internal/component/admin"
	configcomponent "github.com/bartdeboer/ctgbot/internal/component/config"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

const cliSurfaceType = "app"

type componentRegisterCommand struct {
	Component   string
	RuntimeKind string
	HomePath    string
}

type componentUnregisterCommand struct {
	Component string
}

type componentRunCommand struct {
	Component   string
	RuntimeKind string
	HomePath    string
	Args        []string
}

type cliCommandSurface struct {
	service *service
}

var _ component.Component = (*cliCommandSurface)(nil)
var _ component.CommandSurface = (*cliCommandSurface)(nil)

// CLICommandSurfaces returns app-owned command surfaces that can be mounted by
// the local CLI adapter. The CLI is just another protocol adapter here: these
// surfaces still execute through the app service instead of reaching around it.
func (s *service) CLICommandSurfaces(ctx context.Context) ([]component.CommandSurface, error) {
	_ = ctx
	if s == nil {
		return nil, nil
	}
	surfaces := []component.CommandSurface{
		componentadmin.New(s.Repository(), s),
		&cliCommandSurface{service: s},
	}
	if config := s.AppConfig(); config != nil {
		configSurface, err := configcomponent.New(config)
		if err != nil {
			return nil, err
		}
		if configSurface != nil {
			surfaces = append(surfaces, configSurface)
		}
	}
	return surfaces, nil
}

func (s *cliCommandSurface) Type() string { return cliSurfaceType }

func (s *cliCommandSurface) CommandDefinitions() []commandengine.Definition {
	definitions := []commandengine.Definition{
		{
			Pattern:               "component register <component>",
			Help:                  "Register a component instance",
			Build:                 buildComponentRegisterCommand,
			Sources:               []commandengine.Source{commandengine.SourceCLI},
			Policy:                simplerbac.Any(simplerbac.RoleRoot),
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "component unregister <component>",
			Help:    "Unregister a component instance",
			Build: func(req *clir.Request) (any, error) {
				componentRef := strings.TrimSpace(req.Params["component"])
				if componentRef == "" {
					return nil, fmt.Errorf("missing component ref")
				}
				return componentUnregisterCommand{Component: componentRef}, nil
			},
			Sources: []commandengine.Source{commandengine.SourceCLI},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
		},
		{
			Pattern: "component <component>",
			Help:    "Run a registered component CLI command",
			Build:   buildComponentRunCommand,
			Sources: []commandengine.Source{commandengine.SourceCLI},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
		},
	}
	definitions = append(definitions, chatCLICommandDefinitions()...)
	return definitions
}

func (s *cliCommandSurface) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := commandengine.Register[componentRegisterCommand](registry, s.handleComponentRegister); err != nil {
		return err
	}
	if err := commandengine.Register[componentUnregisterCommand](registry, s.handleComponentUnregister); err != nil {
		return err
	}
	if err := commandengine.Register[componentRunCommand](registry, s.handleComponentRun); err != nil {
		return err
	}
	return registerChatCLICommandHandlers(registry, s)
}

func buildComponentRegisterCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("component register", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	runtimeKind := fs.String("runtime", "", "Runtime kind for this registered component")
	homePath := fs.String("home", "", "Optional host component home override")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	componentRef := strings.TrimSpace(req.Params["component"])
	if componentRef == "" {
		return nil, fmt.Errorf("missing component ref")
	}
	return componentRegisterCommand{Component: componentRef, RuntimeKind: strings.TrimSpace(*runtimeKind), HomePath: strings.TrimSpace(*homePath)}, nil
}

func buildComponentRunCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("component", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	runtimeKind := fs.String("runtime", "", "Runtime kind for this component registration")
	homePath := fs.String("home", "", "Optional host component home override")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	componentRef := strings.TrimSpace(req.Params["component"])
	if componentRef == "" {
		return nil, fmt.Errorf("missing component ref")
	}
	return componentRunCommand{Component: componentRef, RuntimeKind: strings.TrimSpace(*runtimeKind), HomePath: strings.TrimSpace(*homePath), Args: fs.Args()}, nil
}

func (s *cliCommandSurface) handleComponentRegister(ctx context.Context, req commandengine.Request, cmd componentRegisterCommand) (commandengine.Result, error) {
	_ = req
	if s == nil || s.service == nil {
		return commandengine.Result{}, fmt.Errorf("missing app service")
	}
	result, err := s.service.RegisterComponent(ctx, cmd.Component, cmd.RuntimeKind, cmd.HomePath)
	if err != nil {
		return commandengine.Result{}, err
	}
	registration := result.Component
	lines := []string{
		"component registered",
		fmt.Sprintf("id: %s", registration.ID),
		fmt.Sprintf("ref: %s", registration.Ref()),
		fmt.Sprintf("runtime: %s", registration.Runtime),
		fmt.Sprintf("home_path: %s", registration.HomePath),
		fmt.Sprintf("host_home: %s", result.HostHomePath),
		fmt.Sprintf("runtime_home: %s", result.RuntimeHomePath),
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (s *cliCommandSurface) handleComponentUnregister(ctx context.Context, req commandengine.Request, cmd componentUnregisterCommand) (commandengine.Result, error) {
	_ = req
	if s == nil || s.service == nil {
		return commandengine.Result{}, fmt.Errorf("missing app service")
	}
	result, err := s.service.UnregisterComponent(ctx, cmd.Component)
	if err != nil {
		return commandengine.Result{}, err
	}
	lines := []string{
		"component unregistered",
		fmt.Sprintf("id: %s", result.ComponentID),
		fmt.Sprintf("ref: %s", result.ComponentRef),
		fmt.Sprintf("chat_components: %d", result.ChatComponents),
		fmt.Sprintf("inbound_filter_bindings: %d", result.InboundFilterBindings),
		fmt.Sprintf("thread_component_mappings: %d", result.ThreadMappings),
		fmt.Sprintf("thread_component_states: %d", result.ThreadStates),
		fmt.Sprintf("component_removed: %t", result.ComponentRemoved),
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (s *cliCommandSurface) handleComponentRun(ctx context.Context, req commandengine.Request, cmd componentRunCommand) (commandengine.Result, error) {
	_ = req
	if s == nil || s.service == nil {
		return commandengine.Result{}, fmt.Errorf("missing app service")
	}
	result, err := s.service.RunComponentCommand(ctx, ComponentCommandRequest{
		ComponentRef: cmd.Component,
		RuntimeKind:  cmd.RuntimeKind,
		HomePath:     cmd.HomePath,
		Args:         cmd.Args,
	})
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: result.Text}, nil
}
