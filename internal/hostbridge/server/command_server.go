package server

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
)

type CommandExecutor interface {
	Execute(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
}

type CommandExecutorFactory func(clientIdentity string) CommandExecutor
type CommandRequestPreparer func(ctx context.Context, clientIdentity string, req commandengine.Request) (commandengine.Request, error)

type CommandServer struct {
	Executor        CommandExecutor
	ExecutorFactory CommandExecutorFactory
	Prepare         CommandRequestPreparer
}

func NewCommandServer(executor CommandExecutor) *CommandServer {
	return &CommandServer{Executor: executor}
}

func NewCommandServerWithFactory(factory CommandExecutorFactory) *CommandServer {
	return &CommandServer{ExecutorFactory: factory}
}

func (s *CommandServer) HandleCommand(ctx context.Context, clientIdentity string, req hostbridge.CommandRequest) hostbridge.CommandResponse {
	if s == nil {
		return hostbridge.CommandResponse{Error: "hostbridge command executor is unavailable"}
	}
	executor := s.Executor
	if s.ExecutorFactory != nil {
		executor = s.ExecutorFactory(clientIdentity)
	}
	if executor == nil {
		return hostbridge.CommandResponse{Error: "hostbridge command executor is unavailable"}
	}
	if s.Prepare != nil {
		var err error
		req.Request, err = s.Prepare(ctx, clientIdentity, req.Request)
		if err != nil {
			return hostbridge.CommandResponse{Error: err.Error()}
		}
	}
	result, err := executor.Execute(ctx, req.Request)
	if err != nil {
		return hostbridge.CommandResponse{Error: err.Error()}
	}
	return hostbridge.CommandResponse{Result: result}
}
