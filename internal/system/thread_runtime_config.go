package system

import (
	"context"

	threadconfig "github.com/bartdeboer/ctgbot/internal/app/config/thread"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

type threadRuntimeConfigResolver struct {
	storage repository.Storage
}

func (r threadRuntimeConfigResolver) RuntimeThreadConfig(ctx context.Context, threadID modeluuid.UUID) (runtimepkg.ThreadConfig, error) {
	if r.storage == nil || threadID.IsNull() {
		return runtimepkg.ThreadConfig{}, nil
	}
	thread, err := r.storage.Threads().GetByID(ctx, threadID)
	if err != nil || thread == nil {
		return runtimepkg.ThreadConfig{}, err
	}
	return runtimepkg.ThreadConfig{
		Ports: threadconfig.RuntimePortsValue(*thread),
	}, nil
}
