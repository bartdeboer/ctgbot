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
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

var _ component.CommandSurface = (*Component)(nil)
var _ component.LocalCommandSurface = (*Component)(nil)
var _ configsurface.ConfigSurface = (*Component)(nil)

func (c *Component) CommandDefinitions() []commandengine.Definition {
	definitions := []commandengine.Definition{
		claudeCommand("container refresh", RefreshContainer{}, "Delete and recreate the Claude runtime on next turn"),
		claudeCommand("container start", StartContainer{}, "Start the Claude runtime container"),
		claudeCommand("container stop", StopContainer{}, "Stop the Claude runtime container but keep its data"),
		claudeCommand("chat purge", PurgeChat{}, "Reset the Claude conversation and delete the runtime container"),
		claudeCommand("interrupt", InterruptTurn{}, "Interrupt the active Claude turn"),
		claudeCommand("status", Status{}, "Show Claude conversation and runtime status"),
		claudeCommand("model", ModelStatus{}, "Show the Claude model for this thread"),
		{
			Pattern: "model set <model>", Help: "Set the Claude model for this thread",
			Build: func(req *clir.Request) (any, error) {
				model := strings.TrimSpace(req.Params["model"])
				if model == "" {
					return nil, fmt.Errorf("missing model")
				}
				return ModelSet{Model: model}, nil
			},
			Sources: claudeCommandSources(), Policy: claudeCommandPolicy(),
		},
		claudeCommand("model clear", ModelClear{}, "Clear the thread model override"),
	}
	definitions = append(definitions, configsurface.CommandDefinitions(configsurface.DefinitionOptions{
		Sources:       claudeCommandSources(),
		Policy:        claudeCommandPolicy(),
		SupportsUnset: true,
	})...)
	return definitions
}

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := commandengine.RegisterPattern[RefreshContainer](registry, "container refresh", func(ctx context.Context, req commandengine.Request, _ RefreshContainer) (commandengine.Result, error) {
		return c.refresh(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[StartContainer](registry, "container start", func(ctx context.Context, req commandengine.Request, _ StartContainer) (commandengine.Result, error) {
		return c.start(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[StopContainer](registry, "container stop", func(ctx context.Context, req commandengine.Request, _ StopContainer) (commandengine.Result, error) {
		return c.stop(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[PurgeChat](registry, "chat purge", func(ctx context.Context, req commandengine.Request, _ PurgeChat) (commandengine.Result, error) {
		return c.purge(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[InterruptTurn](registry, "interrupt", func(ctx context.Context, req commandengine.Request, _ InterruptTurn) (commandengine.Result, error) {
		return c.interrupt(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[Status](registry, "status", func(ctx context.Context, req commandengine.Request, _ Status) (commandengine.Result, error) {
		return c.status(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[ModelStatus](registry, "model", func(ctx context.Context, req commandengine.Request, _ ModelStatus) (commandengine.Result, error) {
		return c.modelStatus(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[ModelSet](registry, "model set <model>", func(ctx context.Context, req commandengine.Request, cmd ModelSet) (commandengine.Result, error) {
		return c.modelSet(ctx, req, cmd)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[ModelClear](registry, "model clear", func(ctx context.Context, req commandengine.Request, _ ModelClear) (commandengine.Result, error) {
		return c.modelClear(ctx, req)
	}); err != nil {
		return err
	}
	return configsurface.RegisterCommandHandlers(registry, c)
}

func (c *Component) refresh(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, workspacePath, err := c.threadWorkspace(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := c.runtime.Refresh(ctx, workspacePath, thread.ID); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "claude runtime refreshed"}, nil
}

func (c *Component) start(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, workspacePath, err := c.threadWorkspace(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	status, err := c.runtime.Start(ctx, workspacePath, thread.ID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := c.updateThreadState(ctx, thread, func(state *threadState) { state.KeepRunning = boolPtr(true) }); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("container started\nkeep_running: true\ncontainer: %s\nstate: %s", status.Name, status.State)}, nil
}

func (c *Component) stop(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, workspacePath, err := c.threadWorkspace(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := c.runtime.Stop(ctx, workspacePath, thread.ID); err != nil {
		return commandengine.Result{}, err
	}
	if err := c.updateThreadState(ctx, thread, func(state *threadState) { state.KeepRunning = nil }); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "container stopped\nkeep_running: false"}, nil
}

func (c *Component) purge(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, workspacePath, err := c.threadWorkspace(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := c.runtime.Refresh(ctx, workspacePath, thread.ID); err != nil {
		return commandengine.Result{}, err
	}
	if err := c.updateThreadState(ctx, thread, func(state *threadState) { state.KeepRunning = nil }); err != nil {
		return commandengine.Result{}, err
	}
	if err := c.storage.ThreadComponentMappings().DeleteByThreadAndComponent(ctx, thread.ID, c.registration.ID); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "claude conversation purged"}, nil
}

func (c *Component) interrupt(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, workspacePath, err := c.threadWorkspace(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	ok, err := c.runtime.Interrupt(ctx, workspacePath, thread.ID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if !ok {
		return commandengine.Result{Text: "no active run to interrupt"}, nil
	}
	return commandengine.Result{Text: "interrupt requested"}, nil
}

func (c *Component) status(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, workspacePath, err := c.threadWorkspace(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	status, err := c.runtime.Status(ctx, workspacePath, thread.ID)
	if err != nil {
		return commandengine.Result{}, err
	}
	mapping, err := c.storage.ThreadComponentMappings().GetByThreadAndComponent(ctx, thread.ID, c.registration.ID)
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
		"runtime: " + c.runtime.Kind(), "container: " + status.Name, "container_state: " + status.State, "workspace: " + workspacePath,
		"runtime_workspace: " + status.RuntimeWorkspacePath, "runtime_home: " + status.RuntimeHomePath, "provider_session_id: " + providerValue,
		"claude_model: " + settings.Model, "claude_model_source: " + settings.ModelSource,
	}
	if status.ActiveCommandName != "" {
		lines = append(lines, "active_command: "+strings.TrimSpace(status.ActiveCommandName+" "+strings.Join(status.ActiveCommandArgs, " ")))
	}
	for _, notice := range status.RuntimeNotices {
		if notice = strings.TrimSpace(notice); notice != "" {
			lines = append(lines, "runtime_notice: "+notice)
		}
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (c *Component) modelStatus(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, err := c.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	settings, err := c.resolveThreadSettings(ctx, thread)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("claude model: %s\nsource: %s", settings.Model, settings.ModelSource)}, nil
}

func (c *Component) modelSet(ctx context.Context, req commandengine.Request, cmd ModelSet) (commandengine.Result, error) {
	thread, err := c.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	model := strings.TrimSpace(cmd.Model)
	if model == "" {
		return commandengine.Result{}, fmt.Errorf("missing model")
	}
	if err := c.updateThreadState(ctx, thread, func(state *threadState) { state.Model = model }); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "claude model=" + model}, nil
}

func (c *Component) modelClear(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, err := c.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := c.updateThreadState(ctx, thread, func(state *threadState) { state.Model = "" }); err != nil {
		return commandengine.Result{}, err
	}
	settings, err := c.resolveThreadSettings(ctx, thread)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("claude model cleared\nclaude model: %s\nsource: %s", settings.Model, settings.ModelSource)}, nil
}

func (c *Component) thread(ctx context.Context, req commandengine.Request) (*coremodel.Thread, error) {
	return agentcommon.Thread(ctx, c.storage, req, Type)
}

func (c *Component) threadWorkspace(ctx context.Context, req commandengine.Request) (*coremodel.Thread, string, error) {
	return agentcommon.ThreadWorkspace(ctx, c.storage, c.resolveWorkspace, req, Type)
}

func claudeCommand(pattern string, command any, help string) commandengine.Definition {
	return commandengine.Definition{Pattern: pattern, Help: help, Build: func(req *clir.Request) (any, error) { _ = req; return command, nil }, Sources: claudeCommandSources(), Policy: claudeCommandPolicy(), InstructionVisibility: claudeInstructionVisibility(pattern)}
}

func claudeInstructionVisibility(pattern string) commandengine.InstructionVisibility {
	switch commandengine.NormalizePattern(pattern) {
	case "container refresh", "chat purge", "interrupt", "status":
		return commandengine.InstructionImportant
	default:
		return commandengine.InstructionDiscoverable
	}
}

func claudeCommandSources() []commandengine.Source {
	return []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge}
}
func claudeCommandPolicy() simplerbac.Rule {
	return simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser)
}
