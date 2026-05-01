package commandengine

import "context"

type CommandExecutor interface {
	Execute(ctx context.Context, req Request) (Result, error)
}
