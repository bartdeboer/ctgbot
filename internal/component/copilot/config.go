package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

type ComponentConfig struct {
	Image             string `json:"image"`
	Dockerfile        string `json:"dockerfile"`
	Model             string `json:"model"`
	SessionTimeoutSec int    `json:"session_timeout_sec"`
}

func loadConfig(profile string) (ComponentConfig, error) {
	config := ComponentConfig{SessionTimeoutSec: DefaultSessionTimeoutSec}
	body, err := os.ReadFile(filepath.Join(profile, "component.json"))
	if err != nil && !os.IsNotExist(err) {
		return config, fmt.Errorf("read copilot component config: %w", err)
	}
	if err == nil {
		if err := json.Unmarshal(body, &config); err != nil {
			return config, fmt.Errorf("parse copilot component config: %w", err)
		}
	}
	config.Image, config.Dockerfile, config.Model = strings.TrimSpace(config.Image), strings.TrimSpace(config.Dockerfile), strings.TrimSpace(config.Model)
	if config.SessionTimeoutSec <= 0 {
		config.SessionTimeoutSec = DefaultSessionTimeoutSec
	}
	if config.SessionTimeoutSec > 7*24*60*60 {
		return config, fmt.Errorf("copilot session timeout exceeds seven days")
	}
	return config, nil
}

type threadState struct {
	Model       string `json:"model,omitempty"`
	KeepRunning bool   `json:"keep_running,omitempty"`
}

func (c *Component) stateStore() agentcommon.JSONStateStore[threadState] {
	return agentcommon.JSONStateStore[threadState]{Storage: c.Storage, ComponentID: c.Registration.ID, Label: Type,
		Clean:  func(s threadState) threadState { s.Model = strings.TrimSpace(s.Model); return s },
		IsZero: func(s threadState) bool { return s.Model == "" && !s.KeepRunning },
	}
}

func (c *Component) settings(ctx context.Context, thread *coremodel.Thread) (threadState, error) {
	if thread == nil {
		return threadState{}, fmt.Errorf("missing copilot thread")
	}
	_, state, err := c.stateStore().Load(ctx, thread.ID)
	if err != nil {
		return state, err
	}
	state.Model = agentcommon.FirstNonEmpty(state.Model, c.config.Model)
	return state, nil
}

func (c *Component) SetKeepRunning(ctx context.Context, thread *coremodel.Thread, keep *bool) error {
	if thread == nil {
		return fmt.Errorf("missing copilot thread")
	}
	return c.stateStore().Update(ctx, thread.ID, func(state *threadState) { state.KeepRunning = keep != nil && *keep })
}
func (c *Component) ThreadSandboxKeepRunning(ctx context.Context, req component.ThreadSandboxRequest) (bool, error) {
	settings, err := c.settings(ctx, &req.Thread)
	return settings.KeepRunning, err
}
func (c *Component) SetThreadSandboxKeepRunning(ctx context.Context, req component.ThreadSandboxRequest, keep *bool) error {
	return c.SetKeepRunning(ctx, &req.Thread, keep)
}
