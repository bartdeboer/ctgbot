package claude

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/buildassets"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

var _ component.CommandSurface = (*Component)(nil)
var _ component.CommandDescriptionSurface = (*Component)(nil)
var _ component.LocalCommandSurface = (*Component)(nil)
var _ configsurface.ConfigSurface = (*Component)(nil)
var _ agentcommon.KeepRunningSetter = (*Component)(nil)

func (c *Component) CommandDefinitions() []commandengine.Definition {
	definitions := agentcommon.AgentCommandDefinitions(agentcommon.AgentCommandOptions{Name: "Claude"})
	definitions = append(definitions, configsurface.CommandDefinitions(configsurface.DefinitionOptions{
		Sources:       agentcommon.AgentCommandSources(),
		Policy:        agentcommon.AgentCommandPolicy(),
		SupportsUnset: true,
	})...)
	return definitions
}

func (c *Component) CommandDescriptions() []commandengine.Description {
	return agentcommon.AgentCommandDescriptions("Claude")
}

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) SetKeepRunning(ctx context.Context, thread *coremodel.Thread, keepRunning *bool) error {
	return c.updateThreadState(ctx, thread, func(state *threadState) { state.KeepRunning = keepRunning })
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	core := &agentcommon.Core{}
	if c != nil {
		core = &c.Core
	}
	statusFn := func(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
		return c.status(ctx, req)
	}
	if err := core.RegisterAgentCommandHandlers(registry, Type, c, statusFn); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[agentcommon.Compact](registry, "compact", func(ctx context.Context, req commandengine.Request, cmd agentcommon.Compact) (commandengine.Result, error) {
		return commandengine.Result{PassthroughPrompt: providerSlashCommand("/compact", cmd.Text)}, nil
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[agentcommon.Goal](registry, "goal", func(ctx context.Context, req commandengine.Request, cmd agentcommon.Goal) (commandengine.Result, error) {
		return commandengine.Result{PassthroughPrompt: providerSlashCommand("/goal", cmd.Text)}, nil
	}); err != nil {
		return err
	}
	return configsurface.RegisterCommandHandlers(registry, c)
}

func providerSlashCommand(name string, text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return name
	}
	return name + " " + text
}

func (c *Component) thread(ctx context.Context, req commandengine.Request) (*coremodel.Thread, error) {
	return agentcommon.Thread(ctx, c.Storage, req, Type)
}

func (c *Component) status(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, workspacePath, err := agentcommon.ThreadWorkspace(ctx, c.Storage, c.ResolveWorkspace, req, Type)
	if err != nil {
		return commandengine.Result{}, err
	}
	status, err := c.Runtime.Status(ctx, workspacePath, thread.ID)
	if err != nil {
		return commandengine.Result{}, err
	}
	mapping, err := c.Storage.ThreadComponentMappings().GetByThreadAndComponent(ctx, thread.ID, c.Registration.ID)
	if err != nil {
		return commandengine.Result{}, err
	}
	providerValue := "(none)"
	if mapping != nil && strings.TrimSpace(mapping.ComponentThreadID) != "" {
		providerValue = strings.TrimSpace(mapping.ComponentThreadID)
	}
	settings, err := c.resolveThreadSettings(ctx, thread)
	if err != nil {
		return commandengine.Result{}, err
	}
	lines := []string{
		"ctgbot_version: " + buildassets.Version(),
		"chat_id: " + thread.ChatID.String(), "thread_id: " + thread.ID.String(), fmt.Sprintf("keep_running: %t", settings.KeepRunning),
		"runtime: " + c.Runtime.Kind(), "container: " + status.Name, "container_state: " + status.State, "workspace: " + workspacePath,
		"runtime_workspace: " + status.RuntimeWorkspacePath, "runtime_profile: " + status.RuntimeProfilePath, "runtime_home: " + status.RuntimeHomePath, "provider_session_id: " + providerValue,
		"claude_model: " + settings.Model, "claude_model_source: " + settings.ModelSource,
	}
	if status.ActiveCommandName != "" {
		lines = append(lines, "active_command: "+strings.TrimSpace(status.ActiveCommandName+" "+strings.Join(status.ActiveCommandArgs, " ")))
	}
	if len(status.Ports) > 0 {
		lines = append(lines, "runtime_ports: "+strings.Join(status.Ports, ", "))
	}
	for _, notice := range status.RuntimeNotices {
		if notice = strings.TrimSpace(notice); notice != "" {
			lines = append(lines, "runtime_notice: "+notice)
		}
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}
