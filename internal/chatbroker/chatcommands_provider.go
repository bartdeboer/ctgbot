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

func (p *ChatCommandsProvider) SendText(ctx context.Context, msg messenger.OutgoingMessage) error {
	if p == nil || p.Broker == nil {
		return fmt.Errorf("missing broker")
	}
	return p.Broker.SendText(ctx, msg)
}

func (p *ChatCommandsProvider) SendFile(ctx context.Context, file messenger.OutgoingFile) error {
	if p == nil || p.Broker == nil {
		return fmt.Errorf("missing broker")
	}
	return p.Broker.SendFile(ctx, file)
}

func (p *ChatCommandsProvider) StartSession(ctx context.Context, chatID modeluuid.UUID, workspace string, replace bool) (chatcommands.SessionInfo, error) {
	if p == nil || p.Broker == nil {
		return chatcommands.SessionInfo{}, fmt.Errorf("missing broker")
	}
	thread, err := p.Broker.StartSession(ctx, chatID, nil, workspace, replace)
	if err != nil {
		return chatcommands.SessionInfo{}, err
	}
	if thread == nil {
		return chatcommands.SessionInfo{}, fmt.Errorf("session was not created")
	}
	return chatcommands.SessionInfo{
		ThreadID:  thread.ID,
		Container: thread.ContainerName(p.Broker.Config),
		Workspace: thread.WorkspaceHost,
	}, nil
}

func (p *ChatCommandsProvider) StopActiveSession(ctx context.Context, threadID modeluuid.UUID) error {
	thread, err := p.threadByID(ctx, threadID)
	if err != nil {
		return err
	}
	return p.Broker.StopSession(ctx, thread)
}

func (p *ChatCommandsProvider) RefreshActiveSession(ctx context.Context, threadID modeluuid.UUID) error {
	thread, err := p.threadByID(ctx, threadID)
	if err != nil {
		return err
	}
	return p.Broker.RefreshSession(ctx, thread)
}

func (p *ChatCommandsProvider) PurgeActiveSession(ctx context.Context, threadID modeluuid.UUID) error {
	thread, err := p.threadByID(ctx, threadID)
	if err != nil {
		return err
	}
	return p.Broker.PurgeSession(ctx, thread)
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
