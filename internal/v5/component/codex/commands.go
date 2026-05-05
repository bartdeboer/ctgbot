package codex

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/go-clir"
)

var _ component.CommandSurface = (*Component)(nil)

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
	return []commandengine.Definition{
		codexCommand("codex.refresh", schemacommands.RefreshContainer{}, "Delete and recreate the Codex runtime on next turn", []string{
			"codex refresh",
			"codex container refresh",
		}),
		codexCommand("codex.container-start", schemacommands.StartContainer{}, "Start the Codex runtime container", []string{
			"codex container start",
		}),
		codexCommand("codex.container-stop", schemacommands.StopContainer{}, "Stop the Codex runtime container but keep its data", []string{
			"codex container stop",
		}),
		codexCommand("codex.purge", schemacommands.PurgeChat{}, "Reset the Codex conversation and delete the runtime container", []string{
			"codex purge",
			"codex chat purge",
		}),
		codexCommand("codex.interrupt", schemacommands.InterruptTurn{}, "Interrupt the active Codex turn", []string{
			"codex interrupt",
		}),
		codexCommand("codex.status", schemacommands.Status{}, "Show Codex conversation and runtime status", []string{
			"codex status",
		}),
		codexCommand("codex.model-status", schemacommands.ModelStatus{}, "Show the Codex model for this thread", []string{
			"codex model",
		}),
		codexCommand("codex.model-list", schemacommands.ModelList{}, "List suggested Codex models", []string{
			"codex model list",
		}),
		{
			ID:      "codex.model-set",
			Sources: codexCommandSources(),
			Policy:  codexCommandPolicy(),
			Routes: []commandengine.Route{{
				Pattern: "codex model set <model>",
				Help:    "Set the Codex model for this thread",
				Build: func(req *clir.Request) (any, error) {
					model := strings.TrimSpace(req.Params["model"])
					if model == "" {
						return nil, fmt.Errorf("missing model")
					}
					return schemacommands.ModelSet{Model: model}, nil
				},
			}},
		},
		codexCommand("codex.model-clear", schemacommands.ModelClear{}, "Clear the thread model override", []string{
			"codex model clear",
		}),
		codexCommand("codex.model-effort-status", schemacommands.ModelEffortStatus{}, "Show the Codex reasoning effort for this thread", []string{
			"codex model effort",
		}),
		codexCommand("codex.model-effort-list", schemacommands.ModelEffortList{}, "List suggested Codex reasoning efforts", []string{
			"codex model effort list",
		}),
		{
			ID:      "codex.model-effort-set",
			Sources: codexCommandSources(),
			Policy:  codexCommandPolicy(),
			Routes: []commandengine.Route{{
				Pattern: "codex model effort set <effort>",
				Help:    "Set the Codex reasoning effort for this thread",
				Build: func(req *clir.Request) (any, error) {
					effort := strings.TrimSpace(req.Params["effort"])
					if effort == "" {
						return nil, fmt.Errorf("missing reasoning effort")
					}
					return schemacommands.ModelEffortSet{Effort: effort}, nil
				},
			}},
		},
		codexCommand("codex.model-effort-clear", schemacommands.ModelEffortClear{}, "Clear the thread reasoning effort override", []string{
			"codex model effort clear",
		}),
	}
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := commandengine.Register[schemacommands.RefreshContainer](registry, func(ctx context.Context, req commandengine.Request, _ schemacommands.RefreshContainer) (commandengine.Result, error) {
		return c.refresh(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.StartContainer](registry, func(ctx context.Context, req commandengine.Request, _ schemacommands.StartContainer) (commandengine.Result, error) {
		return c.start(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.StopContainer](registry, func(ctx context.Context, req commandengine.Request, _ schemacommands.StopContainer) (commandengine.Result, error) {
		return c.stop(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.PurgeChat](registry, func(ctx context.Context, req commandengine.Request, _ schemacommands.PurgeChat) (commandengine.Result, error) {
		return c.purge(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.InterruptTurn](registry, func(ctx context.Context, req commandengine.Request, _ schemacommands.InterruptTurn) (commandengine.Result, error) {
		return c.interrupt(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.Status](registry, func(ctx context.Context, req commandengine.Request, _ schemacommands.Status) (commandengine.Result, error) {
		return c.status(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.ModelStatus](registry, func(ctx context.Context, req commandengine.Request, _ schemacommands.ModelStatus) (commandengine.Result, error) {
		return c.modelStatus(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.ModelList](registry, func(ctx context.Context, req commandengine.Request, _ schemacommands.ModelList) (commandengine.Result, error) {
		return c.modelList(ctx)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.ModelSet](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.ModelSet) (commandengine.Result, error) {
		return c.modelSet(ctx, req, cmd)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.ModelClear](registry, func(ctx context.Context, req commandengine.Request, _ schemacommands.ModelClear) (commandengine.Result, error) {
		return c.modelClear(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.ModelEffortStatus](registry, func(ctx context.Context, req commandengine.Request, _ schemacommands.ModelEffortStatus) (commandengine.Result, error) {
		return c.modelEffortStatus(ctx, req)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.ModelEffortList](registry, func(ctx context.Context, req commandengine.Request, _ schemacommands.ModelEffortList) (commandengine.Result, error) {
		return c.modelEffortList(ctx)
	}); err != nil {
		return err
	}
	if err := commandengine.Register[schemacommands.ModelEffortSet](registry, func(ctx context.Context, req commandengine.Request, cmd schemacommands.ModelEffortSet) (commandengine.Result, error) {
		return c.modelEffortSet(ctx, req, cmd)
	}); err != nil {
		return err
	}
	return commandengine.Register[schemacommands.ModelEffortClear](registry, func(ctx context.Context, req commandengine.Request, _ schemacommands.ModelEffortClear) (commandengine.Result, error) {
		return c.modelEffortClear(ctx, req)
	})
}

func (c *Component) refresh(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, workspacePath, err := c.threadWorkspace(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := c.runtime.Refresh(ctx, workspacePath, thread.ID); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "conversation runtime refreshed"}, nil
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
	return commandengine.Result{Text: fmt.Sprintf("container started\ncontainer: %s\nstate: %s", status.Name, status.State)}, nil
}

func (c *Component) stop(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, workspacePath, err := c.threadWorkspace(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := c.runtime.Stop(ctx, workspacePath, thread.ID); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "container stopped"}, nil
}

func (c *Component) purge(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, workspacePath, err := c.threadWorkspace(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := c.runtime.Refresh(ctx, workspacePath, thread.ID); err != nil {
		return commandengine.Result{}, err
	}
	if err := c.storage.ThreadComponentMappings().DeleteByThreadAndComponent(ctx, thread.ID, c.registration.ID); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "conversation purged"}, nil
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
	providerThreadID, err := c.storage.ThreadComponentMappings().GetByThreadAndComponent(ctx, thread.ID, c.registration.ID)
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
	model, modelSource := c.effectiveModel(thread)
	effort, effortSource := c.effectiveReasoningEffort(thread)
	lines := []string{
		"chat_id: " + thread.ChatID.String(),
		"thread_id: " + thread.ID.String(),
		"runtime: " + c.runtime.Kind(),
		"container: " + status.Name,
		"container_state: " + status.State,
		"workspace: " + workspacePath,
		"runtime_workspace: " + status.RuntimeWorkspacePath,
		"runtime_home: " + status.RuntimeHomePath,
		"provider_thread_id: " + providerValue,
		"codex_model: " + model,
		"codex_model_source: " + modelSource,
		"codex_reasoning_effort: " + effort,
		"codex_reasoning_effort_source: " + effortSource,
	}
	if status.ActiveCommandName != "" {
		lines = append(lines, "active_command: "+strings.TrimSpace(status.ActiveCommandName+" "+strings.Join(status.ActiveCommandArgs, " ")))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (c *Component) modelStatus(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, err := c.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	model, source := c.effectiveModel(thread)
	return commandengine.Result{Text: fmt.Sprintf("codex model: %s\nsource: %s", model, source)}, nil
}

func (c *Component) modelList(ctx context.Context) (commandengine.Result, error) {
	_ = ctx
	return commandengine.Result{Text: "suggested Codex models:\n" + strings.Join(suggestedCodexModels, "\n")}, nil
}

func (c *Component) modelSet(ctx context.Context, req commandengine.Request, cmd schemacommands.ModelSet) (commandengine.Result, error) {
	thread, err := c.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	thread.CodexModel = strings.TrimSpace(cmd.Model)
	if thread.CodexModel == "" {
		return commandengine.Result{}, fmt.Errorf("missing model")
	}
	if err := c.storage.Threads().Save(ctx, thread); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "codex model=" + thread.CodexModel}, nil
}

func (c *Component) modelClear(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, err := c.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	thread.CodexModel = ""
	if err := c.storage.Threads().Save(ctx, thread); err != nil {
		return commandengine.Result{}, err
	}
	model, source := c.effectiveModel(thread)
	return commandengine.Result{Text: fmt.Sprintf("codex model cleared\ncodex model: %s\nsource: %s", model, source)}, nil
}

func (c *Component) modelEffortStatus(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, err := c.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	effort, source := c.effectiveReasoningEffort(thread)
	return commandengine.Result{Text: fmt.Sprintf("codex reasoning effort: %s\nsource: %s", effort, source)}, nil
}

func (c *Component) modelEffortList(ctx context.Context) (commandengine.Result, error) {
	_ = ctx
	return commandengine.Result{Text: "suggested Codex reasoning efforts:\n" + strings.Join(suggestedCodexReasoningEfforts, "\n")}, nil
}

func (c *Component) modelEffortSet(ctx context.Context, req commandengine.Request, cmd schemacommands.ModelEffortSet) (commandengine.Result, error) {
	thread, err := c.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	thread.CodexReasoningEffort = strings.TrimSpace(cmd.Effort)
	if thread.CodexReasoningEffort == "" {
		return commandengine.Result{}, fmt.Errorf("missing reasoning effort")
	}
	if err := c.storage.Threads().Save(ctx, thread); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "codex reasoning effort=" + thread.CodexReasoningEffort}, nil
}

func (c *Component) modelEffortClear(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, err := c.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	thread.CodexReasoningEffort = ""
	if err := c.storage.Threads().Save(ctx, thread); err != nil {
		return commandengine.Result{}, err
	}
	effort, source := c.effectiveReasoningEffort(thread)
	return commandengine.Result{Text: fmt.Sprintf("codex reasoning effort cleared\ncodex reasoning effort: %s\nsource: %s", effort, source)}, nil
}

func (c *Component) thread(ctx context.Context, req commandengine.Request) (*coremodel.Thread, error) {
	if c == nil || c.storage == nil {
		return nil, fmt.Errorf("missing codex storage")
	}
	threadID := req.Context.ThreadID
	if threadID.IsNull() {
		threadID = req.Context.SandboxID
	}
	if threadID.IsNull() {
		return nil, fmt.Errorf("missing thread id")
	}
	thread, err := c.storage.Threads().GetByID(ctx, threadID)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, fmt.Errorf("thread not found: %s", threadID)
	}
	return thread, nil
}

func (c *Component) threadWorkspace(ctx context.Context, req commandengine.Request) (*coremodel.Thread, string, error) {
	thread, err := c.thread(ctx, req)
	if err != nil {
		return nil, "", err
	}
	chat, err := c.storage.Chats().GetByID(ctx, thread.ChatID)
	if err != nil {
		return nil, "", err
	}
	if chat == nil {
		return nil, "", fmt.Errorf("chat not found: %s", thread.ChatID)
	}
	workspacePath, err := c.resolveWorkspace(ctx, *chat)
	if err != nil {
		return nil, "", err
	}
	return thread, workspacePath, nil
}

func (c *Component) effectiveModel(thread *coremodel.Thread) (string, string) {
	if thread != nil {
		if model := strings.TrimSpace(thread.CodexModel); model != "" {
			return model, "thread"
		}
	}
	if c != nil && c.config != nil {
		if model := strings.TrimSpace(c.config.Codex().Model()); model != "" {
			return model, "global"
		}
	}
	return "(codex default)", "codex"
}

func (c *Component) effectiveReasoningEffort(thread *coremodel.Thread) (string, string) {
	if thread != nil {
		if effort := strings.TrimSpace(thread.CodexReasoningEffort); effort != "" {
			return effort, "thread"
		}
	}
	return "(codex default)", "codex"
}

func codexCommand(id string, command any, help string, patterns []string) commandengine.Definition {
	routes := make([]commandengine.Route, 0, len(patterns))
	for _, pattern := range patterns {
		command := command
		routes = append(routes, commandengine.Route{
			Pattern: pattern,
			Help:    help,
			Build: func(req *clir.Request) (any, error) {
				_ = req
				return command, nil
			},
		})
	}
	return commandengine.Definition{
		ID:      id,
		Sources: codexCommandSources(),
		Policy:  codexCommandPolicy(),
		Routes:  routes,
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
