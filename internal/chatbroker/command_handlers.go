package chatbroker

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

type CommandHandlers struct {
	Broker         *Broker
	RunCommandFunc func(ctx context.Context, req commandengine.Request, cmd schemacommands.RunCommand) (commandengine.Result, error)
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

func (h *CommandHandlers) Stop(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, active, err := h.activeThread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if active == nil {
		return commandengine.Result{Text: "no active conversation"}, nil
	}
	if err := h.Broker.StopSession(ctx, thread); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "conversation stopped"}, nil
}

func (h *CommandHandlers) Status(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	thread, active, err := h.activeThread(ctx, req)
	if err != nil {
		return commandengine.Result{}, err
	}
	if active == nil {
		return commandengine.Result{Text: fmt.Sprintf(
			"no active conversation\nchat_id: %s\nthread_id: %s",
			thread.ChatID,
			thread.ID,
		)}, nil
	}
	return commandengine.Result{Text: fmt.Sprintf(
		"active conversation\nchat_id: %s\nthread_id: %s\ncontainer: %s\nworkspace: %s\ninitialized: %t",
		thread.ChatID,
		thread.ID,
		ThreadContainerName(h.Broker.Config, thread),
		thread.WorkspaceHost,
		thread.Initialized,
	)}, nil
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
