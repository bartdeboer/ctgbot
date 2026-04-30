package chatbroker

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

type CommandHandlers struct {
	Broker         *Broker
	RunCommandFunc func(ctx context.Context, req commandengine.Request, cmd schemacommands.RunCommand) (commandengine.Result, error)
}

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

func NewCommandHandlers(broker *Broker) *CommandHandlers {
	return &CommandHandlers{Broker: broker}
}

func (h *CommandHandlers) PrepareHostbridgeRequest(ctx context.Context, clientIdentity string, req commandengine.Request) (commandengine.Request, error) {
	req.Context.Source = commandengine.SourceHostbridge
	req.Context.Actor = commandengine.Actor{ID: clientIdentity, Roles: []simplerbac.Role{simplerbac.RoleAgent}}
	if !req.Context.ThreadID.IsNull() && !req.Context.ChatID.IsNull() {
		return req, nil
	}
	thread, err := h.threadFromRequest(ctx, req)
	if err != nil {
		return commandengine.Request{}, err
	}
	if thread == nil {
		return req, nil
	}
	req.Context.ThreadID = thread.ID
	req.Context.ChatID = thread.ChatID
	return req, nil
}

func (h *CommandHandlers) RunCommand(ctx context.Context, req commandengine.Request, cmd schemacommands.RunCommand) (commandengine.Result, error) {
	if h != nil && h.RunCommandFunc != nil {
		return h.RunCommandFunc(ctx, req, cmd)
	}
	return commandengine.Result{}, fmt.Errorf("run command handler is not configured")
}

func (h *CommandHandlers) SendMedia(ctx context.Context, req commandengine.Request, cmd schemacommands.SendMedia) (commandengine.Result, error) {
	if h == nil || h.Broker == nil {
		return commandengine.Result{}, fmt.Errorf("missing broker")
	}
	sandboxID := req.Context.SandboxID
	if sandboxID.IsNull() {
		sandboxID = req.Context.ThreadID
	}
	if sandboxID.IsNull() {
		return commandengine.Result{}, fmt.Errorf("missing sandbox id")
	}
	err := h.Broker.SendPayload(ctx, sandboxID, messenger.OutboundPayload{
		Text: messenger.TextMessage{Text: cmd.Caption},
		Attachments: []messenger.Media{{
			Filename:    cmd.Filename,
			ContentType: cmd.ContentType,
			Syntax:      cmd.Syntax,
			Content:     append([]byte(nil), cmd.Content...),
		}},
	})
	return commandengine.Result{}, err
}

func (h *CommandHandlers) ScaffoldHostbridgeAllowedCommand(ctx context.Context, req commandengine.Request, cmd schemacommands.ConfigHostbridgeScaffold) (commandengine.Result, error) {
	if h == nil || h.Broker == nil || h.Broker.Config == nil {
		return commandengine.Result{}, fmt.Errorf("missing broker config")
	}
	chatID := req.Context.ChatID
	if chatID.IsNull() {
		return commandengine.Result{}, fmt.Errorf("missing chat id")
	}
	if err := h.Broker.Config.Chat(chatID).Hostbridge().ScaffoldAllowedCommand(cmd.Alias); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("hostbridge allowed command %q scaffolded", cmd.Alias)}, nil
}

func (h *CommandHandlers) RefreshContainer(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, active, err := h.activeThread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if active == nil {
		return commandengine.Result{Text: "no active conversation"}, nil
	}
	if err := h.Broker.RefreshSession(ctx, thread); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "conversation runtime refreshed"}, nil
}

func (h *CommandHandlers) StartContainer(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, err := h.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	conv, err := h.Broker.StartContainer(ctx, thread)
	if err != nil {
		return commandengine.Result{}, err
	}
	if conv == nil {
		conv = thread
	}
	return commandengine.Result{Text: fmt.Sprintf(
		"container started\nkeep_running: %t\ncontainer: %s",
		conv.KeepRunning,
		ThreadContainerName(h.Broker.Config, conv),
	)}, nil
}

func (h *CommandHandlers) StopContainer(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, err := h.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := h.Broker.StopContainer(ctx, thread); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "container stopped\nkeep_running: false"}, nil
}

func (h *CommandHandlers) PurgeChat(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, active, err := h.activeThread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if active == nil {
		return commandengine.Result{Text: "no active conversation"}, nil
	}
	if err := h.Broker.PurgeSession(ctx, thread); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "conversation purged"}, nil
}

func (h *CommandHandlers) InterruptTurn(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, active, err := h.activeThread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if active == nil {
		return commandengine.Result{Text: "no active conversation"}, nil
	}
	if h.Broker.Config == nil || !h.Broker.Config.Chat(thread.ChatID).InteractiveInterruptEnabled() {
		return commandengine.Result{Text: "interrupt is disabled for this chat"}, nil
	}
	if !h.Broker.interruptThread(thread.ID, h.Broker.sandboxForThread(thread)) {
		return commandengine.Result{Text: "no active run to interrupt"}, nil
	}
	return commandengine.Result{Text: "interrupt requested"}, nil
}

func (h *CommandHandlers) Install(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, err := h.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if h.Broker.Config == nil || !h.Broker.Config.Chat(thread.ChatID).ProcessToolsEnabled() {
		return commandengine.Result{Text: "install is not enabled for this chat"}, nil
	}
	if h.Broker.ProcessActions == nil {
		return commandengine.Result{Text: "install is not available in this runtime"}, nil
	}
	if err := h.Broker.ProcessActions.Install(ctx); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "install completed\ntype /quit to restart"}, nil
}

func (h *CommandHandlers) Upgrade(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, err := h.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if h.Broker.Config == nil || !h.Broker.Config.Chat(thread.ChatID).ProcessToolsEnabled() {
		return commandengine.Result{Text: "upgrade is not enabled for this chat"}, nil
	}
	if h.Broker.ProcessActions == nil {
		return commandengine.Result{Text: "upgrade is not available in this runtime"}, nil
	}
	if err := h.Broker.ProcessActions.Upgrade(ctx); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "upgrade completed\ntype /quit to restart"}, nil
}

func (h *CommandHandlers) Quit(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, err := h.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if h.Broker.Config == nil || !h.Broker.Config.Chat(thread.ChatID).ProcessToolsEnabled() {
		return commandengine.Result{Text: "quit is not enabled for this chat"}, nil
	}
	if h.Broker.ProcessActions == nil {
		return commandengine.Result{Text: "quit is not available in this runtime"}, nil
	}
	if err := h.Broker.ProcessActions.Quit(ctx); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "shutting down ctgbot"}, nil
}

func (h *CommandHandlers) Status(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, active, err := h.activeThread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if active == nil {
		model, source := h.effectiveCodexModel(thread)
		effort, effortSource := h.effectiveCodexReasoningEffort(thread)
		return commandengine.Result{Text: fmt.Sprintf(
			"no active conversation\nchat_id: %s\nthread_id: %s\nkeep_running: %t\ncodex_model: %s\ncodex_model_source: %s\ncodex_reasoning_effort: %s\ncodex_reasoning_effort_source: %s",
			thread.ChatID,
			thread.ID,
			thread.KeepRunning,
			model,
			source,
			effort,
			effortSource,
		)}, nil
	}
	model, source := h.effectiveCodexModel(thread)
	effort, effortSource := h.effectiveCodexReasoningEffort(thread)
	return commandengine.Result{Text: fmt.Sprintf(
		"active conversation\nchat_id: %s\nthread_id: %s\ncontainer: %s\nworkspace: %s\ninitialized: %t\nkeep_running: %t\ncodex_model: %s\ncodex_model_source: %s\ncodex_reasoning_effort: %s\ncodex_reasoning_effort_source: %s",
		thread.ChatID,
		thread.ID,
		ThreadContainerName(h.Broker.Config, thread),
		thread.WorkspaceHost,
		thread.Initialized,
		thread.KeepRunning,
		model,
		source,
		effort,
		effortSource,
	)}, nil
}

func (h *CommandHandlers) ModelStatus(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, err := h.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	model, source := h.effectiveCodexModel(thread)
	return commandengine.Result{Text: fmt.Sprintf("codex model: %s\nsource: %s", model, source)}, nil
}

func (h *CommandHandlers) ModelList(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	return commandengine.Result{Text: "suggested Codex models:\n" + strings.Join(suggestedCodexModels, "\n")}, nil
}

func (h *CommandHandlers) ModelSet(ctx context.Context, req commandengine.Request, cmd schemacommands.ModelSet) (commandengine.Result, error) {
	thread, err := h.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	model := strings.TrimSpace(cmd.Model)
	if model == "" {
		return commandengine.Result{}, fmt.Errorf("missing model")
	}
	if h.Broker == nil || h.Broker.Config == nil {
		return commandengine.Result{}, fmt.Errorf("missing broker config")
	}
	if err := h.Broker.Config.Thread(thread.ChatID, thread.ID).SetCodexModel(ctx, model); err != nil {
		return commandengine.Result{}, err
	}
	thread.CodexModel = model
	return commandengine.Result{Text: "codex model=" + model}, nil
}

func (h *CommandHandlers) ModelClear(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, err := h.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if h.Broker == nil || h.Broker.Config == nil {
		return commandengine.Result{}, fmt.Errorf("missing broker config")
	}
	if err := h.Broker.Config.Thread(thread.ChatID, thread.ID).SetCodexModel(ctx, ""); err != nil {
		return commandengine.Result{}, err
	}
	thread.CodexModel = ""
	model, source := h.effectiveCodexModel(thread)
	return commandengine.Result{Text: fmt.Sprintf("codex model cleared\ncodex model: %s\nsource: %s", model, source)}, nil
}

func (h *CommandHandlers) ModelEffortStatus(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, err := h.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	effort, source := h.effectiveCodexReasoningEffort(thread)
	return commandengine.Result{Text: fmt.Sprintf("codex reasoning effort: %s\nsource: %s", effort, source)}, nil
}

func (h *CommandHandlers) ModelEffortList(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	return commandengine.Result{Text: "suggested Codex reasoning efforts:\n" + strings.Join(suggestedCodexReasoningEfforts, "\n")}, nil
}

func (h *CommandHandlers) ModelEffortSet(ctx context.Context, req commandengine.Request, cmd schemacommands.ModelEffortSet) (commandengine.Result, error) {
	thread, err := h.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	effort := strings.TrimSpace(cmd.Effort)
	if effort == "" {
		return commandengine.Result{}, fmt.Errorf("missing reasoning effort")
	}
	if h.Broker == nil || h.Broker.Config == nil {
		return commandengine.Result{}, fmt.Errorf("missing broker config")
	}
	if err := h.Broker.Config.Thread(thread.ChatID, thread.ID).SetCodexReasoningEffort(ctx, effort); err != nil {
		return commandengine.Result{}, err
	}
	thread.CodexReasoningEffort = effort
	return commandengine.Result{Text: "codex reasoning effort=" + effort}, nil
}

func (h *CommandHandlers) ModelEffortClear(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, err := h.thread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if h.Broker == nil || h.Broker.Config == nil {
		return commandengine.Result{}, fmt.Errorf("missing broker config")
	}
	if err := h.Broker.Config.Thread(thread.ChatID, thread.ID).SetCodexReasoningEffort(ctx, ""); err != nil {
		return commandengine.Result{}, err
	}
	thread.CodexReasoningEffort = ""
	effort, source := h.effectiveCodexReasoningEffort(thread)
	return commandengine.Result{Text: fmt.Sprintf("codex reasoning effort cleared\ncodex reasoning effort: %s\nsource: %s", effort, source)}, nil
}

func (h *CommandHandlers) effectiveCodexModel(thread *Thread) (string, string) {
	if model := strings.TrimSpace(thread.CodexModel); model != "" {
		return model, "thread"
	}
	if h != nil && h.Broker != nil && h.Broker.Config != nil {
		if model := strings.TrimSpace(h.Broker.Config.Codex().Model()); model != "" {
			return model, "global"
		}
	}
	return "(codex default)", "codex"
}

func (h *CommandHandlers) effectiveCodexReasoningEffort(thread *Thread) (string, string) {
	if effort := strings.TrimSpace(thread.CodexReasoningEffort); effort != "" {
		return effort, "thread"
	}
	return "(codex default)", "codex"
}

func (h *CommandHandlers) activeThread(ctx context.Context, req commandengine.Request) (*Thread, *Thread, error) {
	thread, err := h.thread(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	active, err := h.Broker.GetActiveSession(ctx, thread)
	if err != nil {
		return nil, nil, err
	}
	return thread, active, nil
}

func (h *CommandHandlers) thread(ctx context.Context, req commandengine.Request) (*Thread, error) {
	thread, err := h.threadFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, fmt.Errorf("missing thread id")
	}
	return thread, nil
}

func (h *CommandHandlers) threadFromRequest(ctx context.Context, req commandengine.Request) (*Thread, error) {
	if h == nil || h.Broker == nil {
		return nil, fmt.Errorf("missing broker")
	}
	threads := h.Broker.threads()
	if threads == nil {
		return nil, fmt.Errorf("missing storage")
	}
	threadID := req.Context.ThreadID
	if threadID.IsNull() {
		threadID = req.Context.SandboxID
	}
	if threadID.IsNull() {
		return nil, nil
	}
	thread, err := threads.GetByID(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("find thread by id: %w", err)
	}
	if thread == nil {
		return nil, fmt.Errorf("thread not found: %s", threadID)
	}
	return thread, nil
}
