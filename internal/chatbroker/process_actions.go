package chatbroker

import "context"

type ProcessActions interface {
	Install(ctx context.Context) error
	Upgrade(ctx context.Context) error
	Quit(ctx context.Context) error
}
