package copilot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

var (
	_ component.CommandSurface            = (*Component)(nil)
	_ component.CommandDescriptionSurface = (*Component)(nil)
	_ component.LocalCommandSurface       = (*Component)(nil)
	_ configsurface.ConfigSurface         = (*Component)(nil)
)

func (*Component) CommandDefinitions() []commandengine.Definition {
	var definitions []commandengine.Definition
	for _, def := range agentcommon.AgentCommandDefinitions(agentcommon.AgentCommandOptions{Name: "Copilot", ThreadInfo: true}) {
		// These would imply unsupported headless goal/compaction semantics.
		if def.Pattern != "goal" && def.Pattern != "compact" {
			definitions = append(definitions, def)
		}
	}
	return append(definitions, configsurface.CommandDefinitions(configsurface.DefinitionOptions{Sources: agentcommon.AgentCommandSources(), Policy: agentcommon.AgentCommandPolicy(), SupportsUnset: true})...)
}
func (*Component) CommandDescriptions() []commandengine.Description {
	return agentcommon.AgentCommandDescriptions("Copilot")
}
func (*Component) UsesLocalCommandRoutes() bool { return true }
func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	core := &agentcommon.Core{}
	if c != nil {
		core = &c.Core
	}
	if err := core.RegisterAgentCommandHandlers(registry, Type, c, c.status); err != nil {
		return err
	}
	return configsurface.RegisterCommandHandlers(registry, c)
}

func (c *Component) thread(ctx context.Context, req commandengine.Request) (*coremodel.Thread, error) {
	if c == nil {
		return nil, fmt.Errorf("missing copilot component")
	}
	return agentcommon.Thread(ctx, c.Storage, req, Type)
}

func (c *Component) status(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	if c == nil || c.Runtime == nil {
		return commandengine.Result{}, fmt.Errorf("missing copilot runtime")
	}
	thread, workspace, err := agentcommon.ThreadWorkspace(ctx, c.Storage, c.ResolveWorkspace, req, Type)
	if err != nil {
		return commandengine.Result{}, err
	}
	status, err := c.Runtime.Status(ctx, workspace, thread.ID)
	if err != nil {
		return commandengine.Result{}, err
	}
	id, err := c.StoredProviderThreadID(ctx, thread.ID)
	if err != nil {
		return commandengine.Result{}, err
	}
	settings, err := c.settings(ctx, thread)
	if err != nil {
		return commandengine.Result{}, err
	}
	// Do not include active argv, raw provider logs or credentials in status.
	return commandengine.Result{Text: fmt.Sprintf("provider: copilot\nCLI contract: %s (installed version unverified)\nprovider_session_id: %s\nmodel: %s\ncontainer: %s\ncontainer_state: %s\nkeep_running: %t\nauthentication: unverified", CLIVersion, agentcommon.FirstNonEmpty(id, "(none)"), agentcommon.FirstNonEmpty(settings.Model, "(copilot default)"), status.Name, status.State, settings.KeepRunning)}, nil
}

const modelKey = "model"
const keepRunningKey = "container.keep-running"

func (c *Component) ConfigSchema(context.Context, commandengine.Request) (configsurface.ConfigSchema, error) {
	model := "(copilot default)"
	if c != nil && c.config.Model != "" {
		model = c.config.Model
	}
	return configsurface.ConfigSchema{Fields: []configsurface.FieldSchema{
		{Key: modelKey, Help: "Copilot model for this thread (subject to account policy)", Type: configsurface.FieldTypeString, Writable: true, Default: model},
		{Key: keepRunningKey, Help: "Keep the existing sandbox running between turns", Type: configsurface.FieldTypeBool, Writable: true, Default: "false", Options: []string{"true", "false"}},
	}}, nil
}
func (c *Component) ConfigGet(ctx context.Context, req commandengine.Request, key string) (string, error) {
	thread, err := c.thread(ctx, req)
	if err != nil {
		return "", err
	}
	settings, err := c.settings(ctx, thread)
	if err != nil {
		return "", err
	}
	switch configsurface.NormalizeKey(key) {
	case modelKey:
		return agentcommon.FirstNonEmpty(settings.Model, "(copilot default)"), nil
	case keepRunningKey:
		return strconv.FormatBool(settings.KeepRunning), nil
	default:
		return "", fmt.Errorf("unknown copilot config key")
	}
}
func (c *Component) ConfigSet(ctx context.Context, req commandengine.Request, key, value string) error {
	thread, err := c.thread(ctx, req)
	if err != nil {
		return err
	}
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsRune(value, 0) {
		return fmt.Errorf("invalid copilot config value")
	}
	switch configsurface.NormalizeKey(key) {
	case modelKey:
		return c.stateStore().Update(ctx, thread.ID, func(s *threadState) { s.Model = value })
	case keepRunningKey:
		keep, err := configsurface.ParseBool(value)
		if err != nil {
			return err
		}
		return c.SetKeepRunning(ctx, thread, &keep)
	default:
		return fmt.Errorf("unknown copilot config key")
	}
}
func (c *Component) ConfigUnset(ctx context.Context, req commandengine.Request, key string) error {
	thread, err := c.thread(ctx, req)
	if err != nil {
		return err
	}
	switch configsurface.NormalizeKey(key) {
	case modelKey:
		return c.stateStore().Update(ctx, thread.ID, func(s *threadState) { s.Model = "" })
	case keepRunningKey:
		return c.SetKeepRunning(ctx, thread, nil)
	default:
		return fmt.Errorf("unknown copilot config key")
	}
}
