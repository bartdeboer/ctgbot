// Package codex contains the component-model Codex implementation skeleton.
package codex

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/agent"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

const ComponentType = "codex"

type Component struct {
	Agent agent.Agent
}

var _ component.Component = (*Component)(nil)
var _ component.ProfileOwner = (*Component)(nil)
var _ agent.Agent = (*Component)(nil)
var _ agent.PurgingAgent = (*Component)(nil)
var _ agent.SkillInstallingAgent = (*Component)(nil)

func New(agentImpl agent.Agent) *Component {
	return &Component{Agent: agentImpl}
}

func (c *Component) Type() string {
	return ComponentType
}

func (c *Component) Name() string {
	if c == nil || c.Agent == nil {
		return ComponentType
	}
	return c.Agent.Name()
}

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{
		{RelativePath: "auth.json", Required: true, Sensitive: true},
		{RelativePath: "config.toml", Required: false, Sensitive: false},
	}
}

func (c *Component) SetupEnvironment(ctx context.Context, sbx *sandboxengine.Sandbox) error {
	if c == nil || c.Agent == nil {
		return fmt.Errorf("missing codex agent")
	}
	return c.Agent.SetupEnvironment(ctx, sbx)
}

func (c *Component) HandleTurn(ctx context.Context, sbx *sandboxengine.Sandbox, output agent.OutputHandler, providerThreadID string, prompt string, options agent.TurnOptions) (agent.TurnResult, error) {
	if c == nil || c.Agent == nil {
		return agent.TurnResult{}, fmt.Errorf("missing codex agent")
	}
	return c.Agent.HandleTurn(ctx, sbx, output, providerThreadID, prompt, options)
}

func (c *Component) Purge(ctx context.Context, sbx *sandboxengine.Sandbox, providerThreadID string) error {
	if c == nil || c.Agent == nil {
		return fmt.Errorf("missing codex agent")
	}
	purger, ok := c.Agent.(agent.PurgingAgent)
	if !ok {
		return fmt.Errorf("codex agent does not support purge")
	}
	return purger.Purge(ctx, sbx, providerThreadID)
}

func (c *Component) InstallSkill(ctx context.Context, sbx *sandboxengine.Sandbox, skillDir string) error {
	if c == nil || c.Agent == nil {
		return fmt.Errorf("missing codex agent")
	}
	installer, ok := c.Agent.(agent.SkillInstallingAgent)
	if !ok {
		return fmt.Errorf("codex agent does not support skill installation")
	}
	return installer.InstallSkill(ctx, sbx, skillDir)
}
