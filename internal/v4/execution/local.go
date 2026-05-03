package execution

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	v4component "github.com/bartdeboer/ctgbot/internal/v4/component"
	"github.com/bartdeboer/ctgbot/internal/v4/profiles"
)

type LocalFactory struct{}

func (LocalFactory) Driver() string { return "local" }

func (LocalFactory) Create(ctx context.Context, profile profiles.Profile, req CreateRequest) (v4component.Runtime, error) {
	_ = ctx
	_ = req
	return &LocalRuntime{profile: profile}, nil
}

type LocalRuntime struct {
	profile profiles.Profile
}

func (r *LocalRuntime) Driver() string {
	return "local"
}

func (r *LocalRuntime) Profile() profiles.Profile {
	return r.profile
}

func (r *LocalRuntime) ContainerWorkspace() string {
	return "/workspace"
}

func (r *LocalRuntime) StartAuth(ctx context.Context, req v4component.RuntimeAuthRequest) (*sandboxengine.Sandbox, error) {
	_ = ctx
	_ = req
	return nil, fmt.Errorf("local runtime is not implemented yet")
}

func (r *LocalRuntime) StartTurn(ctx context.Context, req v4component.RuntimeTurnRequest) (*sandboxengine.SandboxRuntime, error) {
	_ = ctx
	_ = req
	return nil, fmt.Errorf("local runtime is not implemented yet")
}
