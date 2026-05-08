package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

type threadState struct {
	KeepRunning     *bool  `json:"keep_running,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type resolvedThreadSettings struct {
	KeepRunning           bool
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

func (c *Component) loadThreadState(ctx context.Context, threadID modeluuid.UUID) (*coremodel.ThreadComponentState, threadState, error) {
	if c == nil || c.storage == nil {
		return nil, threadState{}, fmt.Errorf("missing codex storage")
	}
	row, err := c.storage.ThreadComponentStates().GetByThreadAndComponent(ctx, threadID, c.registration.ID)
	if err != nil {
		return nil, threadState{}, err
	}
	if row == nil || strings.TrimSpace(row.StateJSON) == "" {
		return row, threadState{}, nil
	}
	var state threadState
	if err := json.Unmarshal([]byte(row.StateJSON), &state); err != nil {
		return nil, threadState{}, fmt.Errorf("decode codex thread state thread=%s component=%s: %w", threadID, c.registration.ID, err)
	}
	return row, state.clean(), nil
}

func (c *Component) saveThreadState(ctx context.Context, storage repository.Storage, threadID modeluuid.UUID, row *coremodel.ThreadComponentState, state threadState) error {
	if storage == nil {
		return fmt.Errorf("missing storage")
	}
	if threadID.IsNull() {
		return fmt.Errorf("missing thread id")
	}
	state = state.clean()
	if state.isZero() {
		return storage.ThreadComponentStates().DeleteByThreadAndComponent(ctx, threadID, c.registration.ID)
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode codex thread state: %w", err)
	}
	if row == nil {
		row = &coremodel.ThreadComponentState{
			ThreadID:    threadID,
			ComponentID: c.registration.ID,
		}
	}
	row.ThreadID = threadID
	row.ComponentID = c.registration.ID
	row.StateJSON = string(data)
	return storage.ThreadComponentStates().Save(ctx, row)
}

func (c *Component) resolveThreadSettings(ctx context.Context, thread *coremodel.Thread) (resolvedThreadSettings, error) {
	settings := resolvedThreadSettings{
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

func (c *Component) updateThreadState(
	ctx context.Context,
	thread *coremodel.Thread,
	mutate func(state *threadState),
) error {
	if thread == nil {
		return fmt.Errorf("missing thread")
	}
	row, state, err := c.loadThreadState(ctx, thread.ID)
	if err != nil {
		return err
	}
	if mutate != nil {
		mutate(&state)
	}
	return c.storage.Transaction(ctx, func(tx repository.Storage) error {
		return c.saveThreadState(ctx, tx, thread.ID, row, state)
	})
}

func boolPtr(value bool) *bool {
	v := value
	return &v
}
