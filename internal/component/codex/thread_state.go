package codex

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type threadState struct {
	KeepRunning     *bool  `json:"keep_running,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type resolvedThreadSettings struct {
	KeepRunning           bool
	SandboxMode           string
	Model                 string
	ModelSource           string
	ReasoningEffort       string
	ReasoningEffortSource string
}

func (s threadState) clean() threadState {
	if s.KeepRunning != nil && !*s.KeepRunning {
		s.KeepRunning = nil
	}
	s.Model = strings.TrimSpace(s.Model)
	s.ReasoningEffort = strings.TrimSpace(s.ReasoningEffort)
	return s
}

func (s threadState) isZero() bool {
	s = s.clean()
	return s.KeepRunning == nil && s.Model == "" && s.ReasoningEffort == ""
}

func (c *Component) stateStore() agentcommon.JSONStateStore[threadState] {
	return agentcommon.JSONStateStore[threadState]{
		Storage:     c.Storage,
		ComponentID: c.Registration.ID,
		Label:       Type,
		Clean:       func(state threadState) threadState { return state.clean() },
		IsZero:      func(state threadState) bool { return state.isZero() },
	}
}

func (c *Component) loadThreadState(ctx context.Context, threadID modeluuid.UUID) (*coremodel.ThreadComponentState, threadState, error) {
	return c.stateStore().Load(ctx, threadID)
}

func (c *Component) resolveThreadSettings(ctx context.Context, thread *coremodel.Thread) (resolvedThreadSettings, error) {
	settings := resolvedThreadSettings{
		SandboxMode:           DefaultSandboxMode,
		Model:                 "(codex default)",
		ModelSource:           "codex",
		ReasoningEffort:       "(codex default)",
		ReasoningEffortSource: "codex",
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
	if c != nil {
		if mode := strings.TrimSpace(c.componentConfig.SandboxMode); mode != "" {
			settings.SandboxMode = mode
		}
	}
	if model := strings.TrimSpace(state.Model); model != "" {
		settings.Model = model
		settings.ModelSource = "thread_component_state"
	} else if c != nil {
		if model := strings.TrimSpace(c.componentConfig.Model); model != "" {
			settings.Model = model
			settings.ModelSource = "profile"
		} else if c.config != nil {
			if model := strings.TrimSpace(c.config.Codex().Model()); model != "" {
				settings.Model = model
				settings.ModelSource = "global"
			}
		}
	}
	if effort := strings.TrimSpace(state.ReasoningEffort); effort != "" {
		settings.ReasoningEffort = effort
		settings.ReasoningEffortSource = "thread_component_state"
	} else if c != nil {
		if effort := strings.TrimSpace(c.componentConfig.ReasoningEffort); effort != "" {
			settings.ReasoningEffort = effort
			settings.ReasoningEffortSource = "profile"
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

func boolPtr(value bool) *bool {
	v := value
	return &v
}
