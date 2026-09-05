package llamacppagent

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
)

// Sandbox persistence is thread-local and independent of model backend lifetime.
type threadState struct {
	KeepRunning bool `json:"keep_running,omitempty"`
}

var _ component.ThreadSandboxKeepRunning = (*Component)(nil)

func (c *Component) stateStore() agentcommon.JSONStateStore[threadState] {
	return agentcommon.JSONStateStore[threadState]{
		Storage:     c.storage,
		ComponentID: c.registration.ID,
		Label:       Type,
		IsZero:      func(state threadState) bool { return !state.KeepRunning },
	}
}

func (c *Component) ThreadSandboxKeepRunning(ctx context.Context, request component.ThreadSandboxRequest) (bool, error) {
	_, state, err := c.stateStore().Load(ctx, request.Thread.ID)
	return state.KeepRunning, err
}

func (c *Component) SetThreadSandboxKeepRunning(ctx context.Context, request component.ThreadSandboxRequest, keepRunning *bool) error {
	return c.stateStore().Update(ctx, request.Thread.ID, func(state *threadState) {
		state.KeepRunning = keepRunning != nil && *keepRunning
	})
}
