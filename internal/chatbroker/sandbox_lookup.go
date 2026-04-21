package chatbroker

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func (b *Broker) ResolveThreadIDBySandboxID(ctx context.Context, sandboxID modeluuid.UUID) (*modeluuid.UUID, error) {
	if b == nil {
		return nil, fmt.Errorf("missing broker")
	}
	if b.Sessions == nil {
		return nil, fmt.Errorf("missing session store")
	}
	if sandboxID.IsNull() {
		return nil, fmt.Errorf("sandbox id is null")
	}

	thread, err := b.Sessions.FindThreadByID(ctx, sandboxID)
	if err != nil {
		return nil, fmt.Errorf("find thread by sandbox id: %w", err)
	}
	if thread == nil || thread.ID.IsNull() {
		return nil, nil
	}

	threadID := thread.ID
	return &threadID, nil
}
