package commandengine

import "context"

type CommandExecutor interface {
	Execute(ctx context.Context, req Request) (Result, error)
}

type CommandRunner interface {
	Run(ctx context.Context, base Request, argv []string) (Result, error)
}

type CommandRuntime interface {
	CommandExecutor
	CommandRunner
}
