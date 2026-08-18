package server

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/hostbridge/transport"
)

type CommandExecutor interface {
	Execute(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
}

type CommandExecutorFactory func(peer transport.PeerIdentity) CommandExecutor
type CommandRequestPreparer func(ctx context.Context, peer transport.PeerIdentity, req commandengine.Request) (commandengine.Request, error)

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

func (s *CommandServer) HandleCommand(ctx context.Context, peer transport.PeerIdentity, req hostbridge.CommandRequest) hostbridge.CommandResponse {
	if s == nil {
		return hostbridge.CommandResponse{Error: "hostbridge command executor is unavailable"}
	}
	executor := s.Executor
	if s.ExecutorFactory != nil {
		executor = s.ExecutorFactory(peer)
	}
	if executor == nil {
		return hostbridge.CommandResponse{Error: "hostbridge command executor is unavailable"}
	}
	if s.Prepare != nil {
		var err error
		req.Request, err = s.Prepare(ctx, peer, req.Request)
		if err != nil {
			return hostbridge.CommandResponse{Error: err.Error()}
		}
	}
	result, err := executor.Execute(ctx, req.Request)
	resp := hostbridge.CommandResponse{Result: result}
	if execution := result.Execution; execution != nil && execution.ExitCode != 0 {
		// Retain the compatibility error for old gob clients. The execution
		// outcome remains authoritative to updated clients.
		resp.Error = executionFailureMessage(*execution)
	}
	if err != nil {
		if resp.Error == "" {
			resp.Error = err.Error()
		}
	}
	return resp
}

var _ transport.CommandHandler = (*CommandServer)(nil)
