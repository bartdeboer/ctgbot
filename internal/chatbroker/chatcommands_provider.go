package chatbroker

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/chatcommands"
	"github.com/bartdeboer/ctgbot/internal/configcommands"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type ChatCommandsProvider struct {
	Broker *Broker
}

func NewChatCommandsProvider(b *Broker) *ChatCommandsProvider {
	return &ChatCommandsProvider{Broker: b}
}

func (p *ChatCommandsProvider) SendPayload(ctx context.Context, sandboxID modeluuid.UUID, payload messenger.OutboundPayload) error {
	if p == nil || p.Broker == nil {
		return fmt.Errorf("missing broker")
	}
	return p.Broker.SendPayload(ctx, sandboxID, payload)
}

func (p *ChatCommandsProvider) Stop(ctx context.Context, threadID modeluuid.UUID) (string, error) {
	thread, active, err := p.activeThreadByID(ctx, threadID)
	if err != nil {
		return "", err
	}
	if active == nil {
		return "no active conversation", nil
	}
	if err := p.Broker.StopSession(ctx, thread); err != nil {
		return "", err
	}
	return "conversation stopped", nil
}

func (p *ChatCommandsProvider) ResolveThreadIDBySandboxID(ctx context.Context, sandboxID modeluuid.UUID) (*modeluuid.UUID, error) {
	if p == nil || p.Broker == nil {
		return nil, fmt.Errorf("missing broker")
	}
	return p.Broker.ResolveThreadIDBySandboxID(ctx, sandboxID)
}

func (p *ChatCommandsProvider) List(ctx context.Context, threadID modeluuid.UUID, cmdctx chatcommands.CommandContext) (string, error) {
	thread, err := p.threadByID(ctx, threadID)
	if err != nil {
		return "", err
	}
	if p.Broker.ConfigCommands == nil {
		return "", fmt.Errorf("config commands are unavailable")
	}
	policyCtx := configcommands.ContextForChat(p.Broker.Config, thread.ChatID, 0, cmdctx.IsRoot)
	return p.Broker.ConfigCommands.List(policyCtx)
}

func (p *ChatCommandsProvider) Set(ctx context.Context, threadID modeluuid.UUID, cmdctx chatcommands.CommandContext, key, value string) (string, error) {
	thread, err := p.threadByID(ctx, threadID)
	if err != nil {
		return "", err
	}
	if p.Broker.ConfigCommands == nil {
		return "", fmt.Errorf("config commands are unavailable")
	}
	policyCtx := configcommands.ContextForChat(p.Broker.Config, thread.ChatID, 0, cmdctx.IsRoot)
	return p.Broker.ConfigCommands.Set(policyCtx, key, value)
}

func (p *ChatCommandsProvider) RefreshContainer(ctx context.Context, threadID modeluuid.UUID) (string, error) {
	thread, active, err := p.activeThreadByID(ctx, threadID)
	if err != nil {
		return "", err
	}
	if active == nil {
		return "no active conversation", nil
	}
	if err := p.Broker.RefreshSession(ctx, thread); err != nil {
		return "", err
	}
	return "conversation runtime refreshed", nil
}

func (p *ChatCommandsProvider) PurgeChat(ctx context.Context, threadID modeluuid.UUID) (string, error) {
	thread, active, err := p.activeThreadByID(ctx, threadID)
	if err != nil {
		return "", err
	}
	if active == nil {
		return "no active conversation", nil
	}
	if err := p.Broker.PurgeSession(ctx, thread); err != nil {
		return "", err
	}
	return "conversation purged", nil
}

func (p *ChatCommandsProvider) InterruptTurn(ctx context.Context, threadID modeluuid.UUID) (string, error) {
	thread, active, err := p.activeThreadByID(ctx, threadID)
	if err != nil {
		return "", err
	}
	if active == nil {
		return "no active conversation", nil
	}
	if p.Broker.Config == nil || !p.Broker.Config.ChatInteractiveInterruptEnabledByID(thread.ChatID) {
		return "interrupt is disabled for this chat", nil
	}
	if !p.Broker.interruptThread(thread.ID, p.Broker.sandboxForThread(thread)) {
		return "no active run to interrupt", nil
	}
	return "interrupt requested", nil
}

func (p *ChatCommandsProvider) Upgrade(ctx context.Context, threadID modeluuid.UUID) (string, error) {
	thread, err := p.threadByID(ctx, threadID)
	if err != nil {
		return "", err
	}
	if p.Broker.Config == nil || !p.Broker.Config.ChatProcessToolsEnabledByID(thread.ChatID) {
		return "upgrade is not enabled for this chat", nil
	}
	if p.Broker.ProcessActions == nil {
		return "upgrade is not available in this runtime", nil
	}
	if err := p.Broker.ProcessActions.Upgrade(ctx); err != nil {
		return "", err
	}
	return "upgrade completed\ntype /quit to restart", nil
}

func (p *ChatCommandsProvider) Quit(ctx context.Context, threadID modeluuid.UUID) (string, error) {
	thread, err := p.threadByID(ctx, threadID)
	if err != nil {
		return "", err
	}
	if p.Broker.Config == nil || !p.Broker.Config.ChatProcessToolsEnabledByID(thread.ChatID) {
		return "quit is not enabled for this chat", nil
	}
	if p.Broker.ProcessActions == nil {
		return "quit is not available in this runtime", nil
	}
	if err := p.Broker.ProcessActions.Quit(ctx); err != nil {
		return "", err
	}
	return "shutting down ctgbot", nil
}

func (p *ChatCommandsProvider) Status(ctx context.Context, threadID modeluuid.UUID) (string, error) {
	thread, active, err := p.activeThreadByID(ctx, threadID)
	if err != nil {
		return "", err
	}
	// TODO: This exposes internal chat/thread/runtime details. Consider restricting
	// this command to authorized users before adding more sensitive status fields.
	if active == nil {
		return fmt.Sprintf(
			"no active conversation\nchat_id: %s\nthread_id: %s",
			thread.ChatID,
			thread.ID,
		), nil
	}
	msg := fmt.Sprintf(
		"active conversation\nchat_id: %s\nthread_id: %s\ncontainer: %s\nworkspace: %s\ninitialized: %t",
		thread.ChatID,
		thread.ID,
		thread.ContainerName(p.Broker.Config),
		thread.WorkspaceHost,
		thread.Initialized,
	)
	if thread.LastError != "" {
		msg += "\nlast_error: " + thread.LastError
	}
	return msg, nil
}

func (p *ChatCommandsProvider) activeThreadByID(ctx context.Context, threadID modeluuid.UUID) (*Thread, *Thread, error) {
	thread, err := p.threadByID(ctx, threadID)
	if err != nil {
		return nil, nil, err
	}
	active, err := p.Broker.GetActiveSession(ctx, thread)
	if err != nil {
		return nil, nil, err
	}
	return thread, active, nil
}

func (p *ChatCommandsProvider) threadByID(ctx context.Context, threadID modeluuid.UUID) (*Thread, error) {
	if p == nil || p.Broker == nil {
		return nil, fmt.Errorf("missing broker")
	}
	if p.Broker.Sessions == nil {
		return nil, fmt.Errorf("missing session store")
	}
	if threadID.IsNull() {
		return nil, fmt.Errorf("thread id is null")
	}
	thread, err := p.Broker.Sessions.FindThreadByID(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("find thread by id: %w", err)
	}
	if thread == nil {
		return nil, fmt.Errorf("thread not found: %s", threadID)
	}
	return thread, nil
}
