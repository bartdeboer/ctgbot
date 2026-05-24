package codex

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/buildassets"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

var _ component.CommandSurface = (*Component)(nil)
var _ component.LocalCommandSurface = (*Component)(nil)
var _ configsurface.ConfigSurface = (*Component)(nil)

var suggestedCodexModels = []string{
	"gpt-5.5",
	"gpt-5.4",
	"gpt-5.4-mini",
	"gpt-5.3-codex",
	"gpt-5.3-codex-spark",
	"gpt-5.2",
}

var suggestedCodexReasoningEfforts = []string{
	"low",
	"medium",
	"high",
	"xhigh",
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	definitions := []commandengine.Definition{
		codexCommand("container refresh", RefreshContainer{}, "Delete and recreate the Codex runtime on next turn",
			hiddenAlias("refresh"),
		),
		codexCommand("container start", StartContainer{}, "Start the Codex runtime container"),
		codexCommand("container stop", StopContainer{}, "Stop the Codex runtime container but keep its data"),
		codexCommand("chat purge", PurgeChat{}, "Reset the Codex conversation and delete the runtime container",
			hiddenAlias("purge"),
		),
		codexCommand("interrupt", InterruptTurn{}, "Interrupt the active Codex turn"),
		codexCommand("status", Status{}, "Show Codex conversation and runtime status"),
	}
	definitions = append(definitions, configsurface.CommandDefinitions(configsurface.DefinitionOptions{
		Sources:       codexCommandSources(),
		Policy:        codexCommandPolicy(),
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
	return configsurface.RegisterCommandHandlers(registry, c)
}

func (c *Component) refresh(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, workspacePath, err := agentcommon.ThreadWorkspace(ctx, c.Storage, c.ResolveWorkspace, req, Type)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := c.RefreshThreadRuntime(ctx, component.ThreadRuntimeControlRequest{Thread: *thread, WorkspacePath: workspacePath}); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "conversation runtime refreshed"}, nil
}

func (c *Component) start(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, workspacePath, err := agentcommon.ThreadWorkspace(ctx, c.Storage, c.ResolveWorkspace, req, Type)
	if err != nil {
		return commandengine.Result{}, err
	}
	status, err := c.Runtime.Start(ctx, workspacePath, thread.ID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := c.updateThreadState(ctx, thread, func(state *threadState) {
		state.KeepRunning = boolPtr(true)
	}); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("container started\nkeep_running: true\ncontainer: %s\nstate: %s", status.Name, status.State)}, nil
}

func (c *Component) stop(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, workspacePath, err := agentcommon.ThreadWorkspace(ctx, c.Storage, c.ResolveWorkspace, req, Type)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := c.Runtime.Stop(ctx, workspacePath, thread.ID); err != nil {
		return commandengine.Result{}, err
	}
	if err := c.updateThreadState(ctx, thread, func(state *threadState) {
		state.KeepRunning = nil
	}); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "container stopped\nkeep_running: false"}, nil
}

func (c *Component) purge(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, workspacePath, err := agentcommon.ThreadWorkspace(ctx, c.Storage, c.ResolveWorkspace, req, Type)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := c.Runtime.Refresh(ctx, workspacePath, thread.ID); err != nil {
		return commandengine.Result{}, err
	}
	if err := c.updateThreadState(ctx, thread, func(state *threadState) {
		state.KeepRunning = nil
	}); err != nil {
		return commandengine.Result{}, err
	}
	if err := c.Storage.ThreadComponentMappings().DeleteByThreadAndComponent(ctx, thread.ID, c.Registration.ID); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "conversation purged"}, nil
}

func (c *Component) interrupt(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, workspacePath, err := agentcommon.ThreadWorkspace(ctx, c.Storage, c.ResolveWorkspace, req, Type)
	if err != nil {
		return commandengine.Result{}, err
	}
	ok, err := c.Runtime.Interrupt(ctx, workspacePath, thread.ID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if !ok {
		return commandengine.Result{Text: "no active run to interrupt"}, nil
	}
	return commandengine.Result{Text: "interrupt requested"}, nil
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
	providerThreadID, err := c.Storage.ThreadComponentMappings().GetByThreadAndComponent(ctx, thread.ID, c.Registration.ID)
	if err != nil {
		return commandengine.Result{}, err
	}
	providerValue := ""
	if providerThreadID != nil {
		providerValue = strings.TrimSpace(providerThreadID.ComponentThreadID)
	}
	if providerValue == "" {
		providerValue = "(none)"
	}
	settings, err := c.resolveThreadSettings(ctx, thread)
	if err != nil {
		return commandengine.Result{}, err
	}
	lines := []string{
		"ctgbot_version: " + buildassets.Version(),
		"chat_id: " + thread.ChatID.String(),
		"thread_id: " + thread.ID.String(),
		fmt.Sprintf("keep_running: %t", settings.KeepRunning),
		"runtime: " + c.Runtime.Kind(),
		"container: " + status.Name,
		"container_state: " + status.State,
		"workspace: " + workspacePath,
		"runtime_workspace: " + status.RuntimeWorkspacePath,
		"runtime_home: " + status.RuntimeHomePath,
		"provider_thread_id: " + providerValue,
		"codex_model: " + settings.Model,
		"codex_model_source: " + settings.ModelSource,
		"codex_reasoning_effort: " + settings.ReasoningEffort,
		"codex_reasoning_effort_source: " + settings.ReasoningEffortSource,
	}
	if status.ActiveCommandName != "" {
		lines = append(lines, "active_command: "+strings.TrimSpace(status.ActiveCommandName+" "+strings.Join(status.ActiveCommandArgs, " ")))
	}
	for _, notice := range status.RuntimeNotices {
		if strings.TrimSpace(notice) == "" {
			continue
		}
		lines = append(lines, "runtime_notice: "+strings.TrimSpace(notice))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func codexCommand(pattern string, command any, help string, aliases ...commandengine.Route) commandengine.Definition {
	commandAliases := make([]commandengine.Route, 0, len(aliases))
	for _, alias := range aliases {
		commandAliases = append(commandAliases, commandengine.Route{
			Pattern:  alias.Pattern,
			Absolute: alias.Absolute,
			Hidden:   alias.Hidden,
		})
	}
	return commandengine.Definition{
		Pattern:               pattern,
		Help:                  help,
		Build:                 func(req *clir.Request) (any, error) { _ = req; return command, nil },
		Sources:               codexCommandSourcesFor(pattern),
		Policy:                codexCommandPolicy(),
		Aliases:               commandAliases,
		InstructionVisibility: codexInstructionVisibility(pattern),
	}
}

func codexInstructionVisibility(pattern string) commandengine.InstructionVisibility {
	switch commandengine.NormalizePattern(pattern) {
	case "container refresh", "chat purge", "interrupt", "status":
		return commandengine.InstructionImportant
	default:
		return commandengine.InstructionDiscoverable
	}
}

func hiddenAlias(pattern string) commandengine.Route {
	return commandengine.Route{Pattern: pattern, Hidden: true}
}

func codexCommandSourcesFor(pattern string) []commandengine.Source {
	switch commandengine.NormalizePattern(pattern) {
	case "container refresh":
		return []commandengine.Source{commandengine.SourceMessage}
	default:
		return codexCommandSources()
	}
}

func codexCommandSources() []commandengine.Source {
	return []commandengine.Source{
		commandengine.SourceMessage,
		commandengine.SourceHostbridge,
	}
}

func codexCommandPolicy() simplerbac.Rule {
	return simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser)
}
