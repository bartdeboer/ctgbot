package ops

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/app"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

const Type = "ops"

type Service interface {
	ResolveChatRef(ctx context.Context, ref string) (modeluuid.UUID, error)
	AddChatComponent(ctx context.Context, chatID modeluuid.UUID, role coremodel.ChatComponentRole, componentRef string, externalChannelID string) (app.ChatComponentAddResult, error)
	RemoveChatComponent(ctx context.Context, chatID modeluuid.UUID, role coremodel.ChatComponentRole, componentRef string) (app.ChatComponentRemoveResult, error)
	ListChatComponents(ctx context.Context, chatID modeluuid.UUID) ([]app.ChatComponentInfo, error)
}

type Component struct {
	service Service
	config  ConfigStore
}

var _ component.Component = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)
var _ component.LocalCommandSurface = (*Component)(nil)

type componentsAddCommand struct {
	Chat              string
	Role              coremodel.ChatComponentRole
	Component         string
	ExternalChannelID string
}

type componentsRemoveCommand struct {
	Chat      string
	Role      coremodel.ChatComponentRole
	Component string
}

type componentsListCommand struct {
	Chat string
}

func RegisterGobTypes(register func(any)) {
	register(componentsAddCommand{})
	register(componentsRemoveCommand{})
	register(componentsListCommand{})
	register(configGetCommand{})
	register(configSetCommand{})
	register(configUnsetCommand{})
	register(configLayersCommand{})
}

func New(service Service, config ...ConfigStore) *Component {
	var cfg ConfigStore
	if len(config) > 0 {
		cfg = config[0]
	}
	return &Component{service: service, config: cfg}
}

func (c *Component) Type() string { return Type }

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) CommandDefinitions() []commandengine.Definition {
	policy := simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent)
	sources := []commandengine.Source{commandengine.SourceCLI, commandengine.SourceHostbridge, commandengine.SourceMessage}
	return []commandengine.Definition{
		{
			Pattern:               "components add <component>",
			Help:                  "Bind a command component to this chat",
			Build:                 buildComponentsAdd,
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "components remove <component>",
			Help:                  "Remove a component binding from this chat",
			Build:                 buildComponentsRemove,
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "components list",
			Help:                  "List component bindings for this chat",
			Build:                 buildComponentsList,
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "config get <key>",
			Help:                  "Show an effective config value and source",
			Build:                 buildConfigGet,
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "config set <layer> <key> <value>",
			Help:                  "Write a config.d layer value",
			Build:                 buildConfigSet,
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "config unset <layer> <key>",
			Help:                  "Remove a config.d layer value",
			Build:                 buildConfigUnset,
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "config layers",
			Help:                  "List config.d layers",
			Build:                 func(req *clir.Request) (any, error) { _ = req; return configLayersCommand{}, nil },
			Sources:               sources,
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
	}
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := commandengine.RegisterPattern[componentsAddCommand](registry, "components add <component>", c.handleComponentsAdd); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[componentsRemoveCommand](registry, "components remove <component>", c.handleComponentsRemove); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[componentsListCommand](registry, "components list", c.handleComponentsList); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[configGetCommand](registry, "config get <key>", c.handleConfigGet); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[configSetCommand](registry, "config set <layer> <key> <value>", c.handleConfigSet); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[configUnsetCommand](registry, "config unset <layer> <key>", c.handleConfigUnset); err != nil {
		return err
	}
	return commandengine.RegisterPattern[configLayersCommand](registry, "config layers", c.handleConfigLayers)
}

func buildComponentsAdd(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("components add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	chat := fs.String("chat", "", "Chat ref to update; defaults to current chat")
	role := fs.String("role", string(coremodel.ChatComponentRoleCommand), "Binding role: command, agent, source, or relay")
	externalChannelID := fs.String("external-channel-id", "", "External channel id for source bindings")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	componentRef := strings.TrimSpace(req.Params["component"])
	if componentRef == "" {
		return nil, fmt.Errorf("missing component ref")
	}
	resolvedRole, err := parseComponentRole(*role)
	if err != nil {
		return nil, err
	}
	return componentsAddCommand{
		Chat:              strings.TrimSpace(*chat),
		Role:              resolvedRole,
		Component:         componentRef,
		ExternalChannelID: strings.TrimSpace(*externalChannelID),
	}, nil
}

func buildComponentsRemove(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("components remove", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	chat := fs.String("chat", "", "Chat ref to update; defaults to current chat")
	role := fs.String("role", string(coremodel.ChatComponentRoleCommand), "Binding role: command, agent, source, or relay")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	componentRef := strings.TrimSpace(req.Params["component"])
	if componentRef == "" {
		return nil, fmt.Errorf("missing component ref")
	}
	resolvedRole, err := parseComponentRole(*role)
	if err != nil {
		return nil, err
	}
	return componentsRemoveCommand{Chat: strings.TrimSpace(*chat), Role: resolvedRole, Component: componentRef}, nil
}

func buildComponentsList(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("components list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	chat := fs.String("chat", "", "Chat ref to inspect; defaults to current chat")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	return componentsListCommand{Chat: strings.TrimSpace(*chat)}, nil
}

func parseComponentRole(value string) (coremodel.ChatComponentRole, error) {
	switch role := coremodel.ChatComponentRole(strings.TrimSpace(value)); role {
	case coremodel.ChatComponentRoleCommand, coremodel.ChatComponentRoleAgent, coremodel.ChatComponentRoleSource, coremodel.ChatComponentRoleRelay:
		return role, nil
	default:
		return "", fmt.Errorf("invalid component role %q", value)
	}
}

func (c *Component) handleComponentsAdd(ctx context.Context, req commandengine.Request, cmd componentsAddCommand) (commandengine.Result, error) {
	chatID, err := c.resolveChat(ctx, req, cmd.Chat)
	if err != nil {
		return commandengine.Result{}, err
	}
	result, err := c.serviceForUse().AddChatComponent(ctx, chatID, cmd.Role, cmd.Component, cmd.ExternalChannelID)
	if err != nil {
		return commandengine.Result{}, err
	}
	lines := []string{"ops component added", fmt.Sprintf("chat_id: %s", result.Binding.ChatID), fmt.Sprintf("component: %s", result.ComponentRef), fmt.Sprintf("role: %s", result.Binding.Role)}
	if result.Runtime != "" {
		lines = append(lines, fmt.Sprintf("runtime: %s", result.Runtime))
	}
	if result.Binding.ExternalChannelID != "" {
		lines = append(lines, fmt.Sprintf("external_channel_id: %s", result.Binding.ExternalChannelID))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (c *Component) handleComponentsRemove(ctx context.Context, req commandengine.Request, cmd componentsRemoveCommand) (commandengine.Result, error) {
	chatID, err := c.resolveChat(ctx, req, cmd.Chat)
	if err != nil {
		return commandengine.Result{}, err
	}
	result, err := c.serviceForUse().RemoveChatComponent(ctx, chatID, cmd.Role, cmd.Component)
	if err != nil {
		return commandengine.Result{}, err
	}
	if !result.Removed {
		return commandengine.Result{Text: strings.Join([]string{"ops component binding not found", fmt.Sprintf("chat_id: %s", chatID), fmt.Sprintf("component: %s", result.ComponentRef), fmt.Sprintf("role: %s", cmd.Role)}, "\n")}, nil
	}
	return commandengine.Result{Text: strings.Join([]string{"ops component removed", fmt.Sprintf("chat_id: %s", result.Binding.ChatID), fmt.Sprintf("component: %s", result.ComponentRef), fmt.Sprintf("role: %s", result.Binding.Role)}, "\n")}, nil
}

func (c *Component) handleComponentsList(ctx context.Context, req commandengine.Request, cmd componentsListCommand) (commandengine.Result, error) {
	chatID, err := c.resolveChat(ctx, req, cmd.Chat)
	if err != nil {
		return commandengine.Result{}, err
	}
	bindings, err := c.serviceForUse().ListChatComponents(ctx, chatID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if len(bindings) == 0 {
		return commandengine.Result{Text: "ops components\nno component bindings"}, nil
	}
	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].Binding.Role != bindings[j].Binding.Role {
			return bindings[i].Binding.Role < bindings[j].Binding.Role
		}
		return bindings[i].ComponentRef < bindings[j].ComponentRef
	})
	lines := []string{"ops components"}
	for _, binding := range bindings {
		line := fmt.Sprintf("%s\trole=%s", binding.ComponentRef, binding.Binding.Role)
		if binding.Runtime != "" {
			line += fmt.Sprintf("\truntime=%s", binding.Runtime)
		}
		if binding.Binding.ExternalChannelID != "" {
			line += fmt.Sprintf("\texternal_channel_id=%s", binding.Binding.ExternalChannelID)
		}
		lines = append(lines, line)
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (c *Component) resolveChat(ctx context.Context, req commandengine.Request, ref string) (modeluuid.UUID, error) {
	service := c.serviceForUse()
	ref = strings.TrimSpace(ref)
	if ref != "" {
		return service.ResolveChatRef(ctx, ref)
	}
	if !req.Context.ChatID.IsNull() {
		return req.Context.ChatID, nil
	}
	return modeluuid.UUID{}, fmt.Errorf("missing chat; pass --chat when no current chat is available")
}

func (c *Component) serviceForUse() Service {
	if c == nil || c.service == nil {
		return missingService{}
	}
	return c.service
}

type missingService struct{}

func (missingService) ResolveChatRef(context.Context, string) (modeluuid.UUID, error) {
	return modeluuid.UUID{}, fmt.Errorf("missing ops service")
}
func (missingService) AddChatComponent(context.Context, modeluuid.UUID, coremodel.ChatComponentRole, string, string) (app.ChatComponentAddResult, error) {
	return app.ChatComponentAddResult{}, fmt.Errorf("missing ops service")
}
func (missingService) RemoveChatComponent(context.Context, modeluuid.UUID, coremodel.ChatComponentRole, string) (app.ChatComponentRemoveResult, error) {
	return app.ChatComponentRemoveResult{}, fmt.Errorf("missing ops service")
}
func (missingService) ListChatComponents(context.Context, modeluuid.UUID) ([]app.ChatComponentInfo, error) {
	return nil, fmt.Errorf("missing ops service")
}
