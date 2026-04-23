package chatcommands

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type ProviderRunner struct {
	Provider Provider
}

type DispatchRunner struct {
	Host     HostCommandRunner
	Provider Runner
}

func NewProviderRunner(provider Provider) *ProviderRunner {
	return &ProviderRunner{Provider: provider}
}

func NewDispatchRunner(host HostCommandRunner, provider Runner) *DispatchRunner {
	return &DispatchRunner{Host: host, Provider: provider}
}

func (r *DispatchRunner) Execute(ctx context.Context, req Request) (Result, error) {
	if req.Command == nil {
		return Result{}, fmt.Errorf("missing command")
	}
	if cmd, ok := req.Command.(RunCommand); ok {
		if r == nil || r.Host == nil {
			return Result{}, fmt.Errorf("run command runner is unavailable")
		}
		return r.Host.ExecuteRunCommand(ctx, req, cmd)
	}
	if r == nil || r.Provider == nil {
		return Result{}, fmt.Errorf("provider runner is unavailable")
	}
	return r.Provider.Execute(ctx, req)
}

func (r *ProviderRunner) Execute(ctx context.Context, req Request) (Result, error) {
	if r == nil || r.Provider == nil {
		return Result{}, fmt.Errorf("chat command provider is unavailable")
	}
	if req.Command == nil {
		return Result{}, fmt.Errorf("missing command")
	}

	switch cmd := req.Command.(type) {
	case SendMedia:
		if req.SandboxID.IsNull() {
			return Result{}, fmt.Errorf("missing sandbox id")
		}
		err := r.Provider.SendPayload(ctx, req.SandboxID, messenger.OutboundPayload{
			Text: messenger.TextMessage{
				Text: cmd.Caption,
			},
			Attachments: []messenger.Media{{
				Filename:    cmd.Filename,
				ContentType: cmd.ContentType,
				Syntax:      cmd.Syntax,
				Content:     append([]byte(nil), cmd.Content...),
			}},
		})
		return Result{}, err
	case ConfigList:
		threadID, err := r.resolveThreadID(ctx, req)
		if err != nil {
			return Result{}, err
		}
		text, err := r.Provider.List(ctx, threadID, req.Context)
		if err != nil {
			return Result{}, err
		}
		return Result{Text: text}, nil
	case ConfigSet:
		threadID, err := r.resolveThreadID(ctx, req)
		if err != nil {
			return Result{}, err
		}
		text, err := r.Provider.Set(ctx, threadID, req.Context, cmd.Setting, cmd.Value)
		if err != nil {
			return Result{}, err
		}
		return Result{Text: text}, nil
	case RefreshContainer:
		threadID, err := r.resolveThreadID(ctx, req)
		if err != nil {
			return Result{}, err
		}
		text, err := r.Provider.RefreshContainer(ctx, threadID)
		if err != nil {
			return Result{}, err
		}
		return Result{Text: text}, nil
	case PurgeChat:
		threadID, err := r.resolveThreadID(ctx, req)
		if err != nil {
			return Result{}, err
		}
		text, err := r.Provider.PurgeChat(ctx, threadID)
		if err != nil {
			return Result{}, err
		}
		return Result{Text: text}, nil
	case InterruptTurn:
		threadID, err := r.resolveThreadID(ctx, req)
		if err != nil {
			return Result{}, err
		}
		text, err := r.Provider.InterruptTurn(ctx, threadID)
		if err != nil {
			return Result{}, err
		}
		return Result{Text: text}, nil
	case Upgrade:
		threadID, err := r.resolveThreadID(ctx, req)
		if err != nil {
			return Result{}, err
		}
		text, err := r.Provider.Upgrade(ctx, threadID)
		if err != nil {
			return Result{}, err
		}
		return Result{Text: text}, nil
	case Quit:
		threadID, err := r.resolveThreadID(ctx, req)
		if err != nil {
			return Result{}, err
		}
		text, err := r.Provider.Quit(ctx, threadID)
		if err != nil {
			return Result{}, err
		}
		return Result{Text: text}, nil
	case Stop:
		threadID, err := r.resolveThreadID(ctx, req)
		if err != nil {
			return Result{}, err
		}
		text, err := r.Provider.Stop(ctx, threadID)
		if err != nil {
			return Result{}, err
		}
		return Result{Text: text}, nil
	case Status:
		threadID, err := r.resolveThreadID(ctx, req)
		if err != nil {
			return Result{}, err
		}
		text, err := r.Provider.Status(ctx, threadID)
		if err != nil {
			return Result{}, err
		}
		return Result{Text: text}, nil
	case RunCommand:
		return Result{}, fmt.Errorf("run command is not supported by the provider runner")
	default:
		return Result{}, fmt.Errorf("unsupported command type %T", req.Command)
	}
}

func (r *ProviderRunner) resolveThreadID(ctx context.Context, req Request) (modeluuid.UUID, error) {
	if !req.ThreadID.IsNull() {
		return req.ThreadID, nil
	}
	if req.SandboxID.IsNull() {
		return modeluuid.Nil, fmt.Errorf("missing thread id")
	}
	threadID, err := r.Provider.ResolveThreadIDBySandboxID(ctx, req.SandboxID)
	if err != nil {
		return modeluuid.Nil, err
	}
	if threadID == nil || threadID.IsNull() {
		return modeluuid.Nil, fmt.Errorf("missing thread id")
	}
	return *threadID, nil
}
