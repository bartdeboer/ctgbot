package claude

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

type threadState struct {
	KeepRunning *bool  `json:"keep_running,omitempty"`
	Model       string `json:"model,omitempty"`
}

type resolvedThreadSettings struct {
	KeepRunning       bool
	Model             string
	ModelSource       string
	PermissionMode    string
	SessionTimeoutSec int
}

func (s threadState) clean() threadState {
	if s.KeepRunning != nil && !*s.KeepRunning {
		s.KeepRunning = nil
	}
	s.Model = strings.TrimSpace(s.Model)
	return s
}

func (s threadState) isZero() bool {
	s = s.clean()
	return s.KeepRunning == nil && s.Model == ""
}

func (c *Component) stateStore() agentcommon.JSONStateStore[threadState] {
	return agentcommon.JSONStateStore[threadState]{
		Storage:     c.storage,
		ComponentID: c.registration.ID,
		Label:       Type,
		Clean:       func(state threadState) threadState { return state.clean() },
		IsZero:      func(state threadState) bool { return state.isZero() },
	}
}

func (c *Component) loadThreadState(ctx context.Context, threadID modeluuid.UUID) (*coremodel.ThreadComponentState, threadState, error) {
	return c.stateStore().Load(ctx, threadID)
}

func (c *Component) saveThreadState(ctx context.Context, storage repository.Storage, threadID modeluuid.UUID, row *coremodel.ThreadComponentState, state threadState) error {
	return c.stateStore().Save(ctx, storage, threadID, row, state)
}

func (c *Component) resolveThreadSettings(ctx context.Context, thread *coremodel.Thread) (resolvedThreadSettings, error) {
	settings := resolvedThreadSettings{
		Model:             "(claude default)",
		ModelSource:       "claude",
		PermissionMode:    "bypassPermissions",
		SessionTimeoutSec: DefaultSessionTimeoutSec,
	}
	if c != nil {
		if mode := strings.TrimSpace(c.componentConfig.PermissionMode); mode != "" {
			settings.PermissionMode = mode
		}
		if c.componentConfig.SessionTimeoutSec > 0 {
			settings.SessionTimeoutSec = c.componentConfig.SessionTimeoutSec
		}
	}
	if thread == nil {
		return settings, nil
	}
	_, state, err := c.loadThreadState(ctx, thread.ID)
	if err != nil {
		return resolvedThreadSettings{}, err
	}
	if state.KeepRunning != nil && *state.KeepRunning {
		settings.KeepRunning = true
	}
	if model := strings.TrimSpace(state.Model); model != "" {
		settings.Model = model
		settings.ModelSource = "thread_component_state"
	} else if c != nil {
		if model := strings.TrimSpace(c.componentConfig.Model); model != "" {
			settings.Model = model
			settings.ModelSource = "profile"
		}
	}
	return settings, nil
}

func (c *Component) updateThreadState(ctx context.Context, thread *coremodel.Thread, mutate func(state *threadState)) error {
	if thread == nil {
		return fmt.Errorf("missing thread")
	}
	return c.stateStore().Update(ctx, thread.ID, mutate)
}

func boolPtr(value bool) *bool { v := value; return &v }
