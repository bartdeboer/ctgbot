package chatbroker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/agent"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func (b *Broker) GetActiveSession(ctx context.Context, thread *Thread) (*Thread, error) {
	if thread == nil {
		return nil, nil
	}
	if !thread.Active {
		return nil, nil
	}
	return thread, nil
}

func (b *Broker) StartSession(ctx context.Context, chatID modeluuid.UUID, thread *Thread, workspace string, replace bool) (*Thread, error) {
	var out *Thread
	err := b.dispatcher().Run(ctx, b.dispatchKey(chatID, threadIDOrNil(thread)), func(runCtx context.Context) error {
		var err error
		out, err = b.startSession(runCtx, chatID, thread, workspace, replace)
		return err
	})
	return out, err
}

func (b *Broker) startSession(ctx context.Context, chatID modeluuid.UUID, thread *Thread, workspace string, replace bool) (*Thread, error) {
	if chatID.IsNull() {
		return nil, fmt.Errorf("missing chat id")
	}
	if thread == nil {
		thread = &Thread{
			ID:     modeluuid.New(),
			ChatID: chatID,
		}
	}
	if thread.ChatID.IsNull() {
		thread.ChatID = chatID
	}
	if thread.ChatID != chatID {
		return nil, fmt.Errorf("thread chat id mismatch")
	}

	current, err := b.GetActiveSession(ctx, thread)
	if err != nil {
		return nil, err
	}
	if current != nil {
		if !replace {
			return current, nil
		}
		_ = b.sandboxForThread(current).Remove(ctx)
		if b.Sessions != nil {
			current.Active = false
			current.LastError = "replaced by /new"
			_ = b.Sessions.SaveThread(ctx, current)
		}
	}

	conv, err := b.prepareThread(ctx, chatID, thread, workspace)
	if err != nil {
		return nil, err
	}
	if _, _, err := b.prepareRuntime(ctx, conv, true); err != nil {
		return nil, err
	}
	if b.Sessions != nil {
		if err := b.Sessions.SaveThread(ctx, conv); err != nil {
			_ = b.sandboxForThread(conv).Remove(context.Background())
			return nil, err
		}
	}
	return conv, nil
}

func (b *Broker) StopSession(ctx context.Context, thread *Thread) error {
	if thread == nil {
		return nil
	}
	return b.dispatcher().Run(ctx, b.dispatchKey(thread.ChatID, thread.ID), func(runCtx context.Context) error {
		return b.stopSession(runCtx, thread)
	})
}

func (b *Broker) RefreshSession(ctx context.Context, thread *Thread) error {
	if thread == nil {
		return nil
	}
	return b.dispatcher().Run(ctx, b.dispatchKey(thread.ChatID, thread.ID), func(runCtx context.Context) error {
		return b.refreshSession(runCtx, thread)
	})
}

func (b *Broker) PurgeSession(ctx context.Context, thread *Thread) error {
	if thread == nil {
		return nil
	}
	return b.dispatcher().Run(ctx, b.dispatchKey(thread.ChatID, thread.ID), func(runCtx context.Context) error {
		return b.purgeSession(runCtx, thread)
	})
}

func (b *Broker) stopSession(ctx context.Context, conv *Thread) error {
	if conv == nil {
		return nil
	}
	if err := b.sandboxForThread(conv).Remove(ctx); err != nil {
		return err
	}
	if b.Sessions == nil {
		return nil
	}
	conv.Active = false
	conv.LastError = "stopped by /stop"
	return b.Sessions.SaveThread(ctx, conv)
}

func (b *Broker) refreshSession(ctx context.Context, conv *Thread) error {
	if conv == nil {
		return nil
	}
	if err := b.sandboxForThread(conv).Remove(ctx); err != nil {
		return err
	}
	conv.Initialized = false
	conv.LastError = ""
	if _, _, err := b.prepareRuntime(ctx, conv, true); err != nil {
		if b.Sessions != nil {
			conv.LastError = err.Error()
			_ = b.Sessions.SaveThread(ctx, conv)
		}
		return err
	}
	return nil
}

func (b *Broker) purgeSession(ctx context.Context, conv *Thread) error {
	if conv == nil {
		return nil
	}
	if err := b.sandboxForThread(conv).Remove(ctx); err != nil {
		return err
	}
	agentImpl, err := b.agent(conv.AgentProviderType)
	if err != nil {
		return err
	}
	if purgingAgent, ok := agentImpl.(agent.PurgingAgent); ok && strings.TrimSpace(conv.AgentThreadID) != "" {
		if err := purgingAgent.Purge(ctx, b.sandboxForThread(conv), conv.AgentThreadID); err != nil {
			if b.Sessions != nil {
				conv.LastError = err.Error()
				_ = b.Sessions.SaveThread(ctx, conv)
			}
			return err
		}
	}
	conv.Active = false
	conv.Initialized = false
	conv.AgentThreadID = ""
	conv.LastError = ""
	if b.Sessions != nil {
		return b.Sessions.SaveThread(ctx, conv)
	}
	return nil
}

func (b *Broker) PrepareSession(ctx context.Context, conv *Thread) error {
	if conv == nil {
		return fmt.Errorf("missing thread")
	}
	return b.dispatcher().Run(ctx, b.dispatchKey(conv.ChatID, conv.ID), func(runCtx context.Context) error {
		_, _, err := b.prepareRuntime(runCtx, conv, true)
		return err
	})
}

func (b *Broker) HandlePrompt(ctx context.Context, chatID modeluuid.UUID, thread *Thread, prompt string) (PromptOutcome, error) {
	var out PromptOutcome
	err := b.dispatcher().Run(ctx, b.dispatchKey(chatID, threadIDOrNil(thread)), func(runCtx context.Context) error {
		var err error
		out, err = b.handlePrompt(runCtx, chatID, thread, prompt)
		return err
	})
	return out, err
}

func (b *Broker) handlePrompt(ctx context.Context, chatID modeluuid.UUID, thread *Thread, prompt string) (PromptOutcome, error) {
	conv, err := b.GetActiveSession(ctx, thread)
	if err != nil {
		return PromptOutcome{}, err
	}

	started := false
	if conv == nil {
		conv, err = b.startSession(ctx, chatID, thread, "", false)
		if err != nil {
			return PromptOutcome{}, err
		}
		started = true
		if err := b.SendSystemMessage(ctx, conv, fmt.Sprintf("conversation started\ncontainer: %s\nworkspace: %s", conv.ContainerName(b.Config), conv.WorkspaceHost)); err != nil {
			b.logf("send conversation started message failed thread=%s err=%v", conv.ID, err)
		}
	}

	runCtx, cancel := context.WithCancel(ctx)
	b.setActiveRun(conv.ID, cancel)
	defer b.clearActiveRun(conv.ID, cancel)
	defer cancel()

	agent, sbx, err := b.prepareRuntime(runCtx, conv, false)
	if err != nil {
		return PromptOutcome{}, err
	}
	defer func() {
		if stopErr := sbx.Stop(context.Background()); stopErr != nil {
			b.logf("stop conversation sandbox %s failed: %v", conv.ContainerName(b.Config), stopErr)
		}
	}()

	stopTyping := b.startThreadChatAction(runCtx, conv, messenger.ChatActionTyping)
	defer stopTyping()

	result, runErr := agent.HandleTurn(runCtx, sbx, conv.AgentThreadID, prompt)
	if result.ProviderThreadID != "" {
		conv.AgentThreadID = result.ProviderThreadID
	}
	interrupted := errors.Is(runErr, context.Canceled)
	if b.Sessions != nil {
		if interrupted {
			conv.LastError = "interrupted"
		} else if runErr != nil {
			conv.LastError = runErr.Error()
		} else {
			conv.LastError = ""
		}
		_ = b.Sessions.SaveThread(ctx, conv)
	}
	if interrupted {
		return PromptOutcome{Thread: conv, Started: started}, nil
	}

	return PromptOutcome{
		Thread:  conv,
		Started: started,
		Reply:   result.Reply,
	}, runErr
}

func (b *Broker) prepareThread(ctx context.Context, chatID modeluuid.UUID, thread *Thread, workspace string) (*Thread, error) {
	if b.Config == nil {
		return nil, fmt.Errorf("missing config")
	}
	if chatID.IsNull() {
		return nil, fmt.Errorf("missing chat id")
	}
	if thread == nil {
		thread = &Thread{
			ID:     modeluuid.New(),
			ChatID: chatID,
		}
	}
	if thread.ID.IsNull() {
		thread.ID = modeluuid.New()
	}
	thread.ChatID = chatID

	if err := b.Config.EnsurePaths(); err != nil {
		return nil, err
	}
	if _, err := b.Config.EnsureChatRuntimePaths(chatID); err != nil {
		return nil, err
	}

	workspaceHostPath, err := b.Config.ResolveChatWorkspaceHostPathByID(chatID, workspace)
	if err != nil {
		return nil, err
	}

	thread.Active = true
	thread.AgentProviderType = b.defaultAgentName()
	thread.RuntimeName = b.Config.ThreadContainerName(thread.ID)
	thread.WorkspaceHost = workspaceHostPath
	thread.HomeHost = b.Config.ChatCodexProfileHostPathByID(thread.ChatID)
	thread.ContainerWorkspace = b.Config.DockerContainerWorkspacePath()
	thread.ContainerHome = b.Config.DockerContainerHomePath()
	thread.Initialized = false
	thread.AgentThreadID = ""
	thread.LastError = ""

	if err := b.sandboxForThread(thread).Remove(ctx); err != nil {
		b.logf("ignoring stale sandbox cleanup error for %s: %v", thread.ContainerName(b.Config), err)
	}
	b.logf("thread prepared name=%s workspace=%s", thread.ContainerName(b.Config), thread.WorkspaceHost)
	return thread, nil
}
