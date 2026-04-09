package chatbroker

import "context"

type ProcessActions interface {
	Upgrade(ctx context.Context) error
	Quit(ctx context.Context) error
}
