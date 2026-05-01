// Package codex contains the component-model Codex implementation skeleton.
//
// This package is intentionally separate from internal/agent/codexengine. The
// existing agent path can keep serving the current broker while a future broker
// can build on component/codex directly.
package codex

import (
	"strings"

	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/ctgbot/internal/v2/component"
	"github.com/bartdeboer/ctgbot/internal/v2/repository"
)

const ComponentType = "codex"

type Config struct {
	ProfileName          string
	ProfileHostPath      string
	ProfileContainerPath string
	WorkspaceRoot        string
	Image                string
	SandboxManager       sandboxengine.RuntimeManager
	StateStore           repository.ThreadComponentStateRepository
}

type Component struct {
	Config Config
}

var _ component.Component = (*Component)(nil)
var _ component.ProfileOwner = (*Component)(nil)
var _ component.Authenticator = (*Component)(nil)
var _ component.Agent = (*Component)(nil)
var _ component.Profiled = (*Component)(nil)

func New(config ...Config) *Component {
	c := &Component{}
	if len(config) > 0 {
		c.Config = config[0]
	}
	return c
}

func (c *Component) Type() string {
	return ComponentType
}

func (c *Component) ProfileName() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.Config.ProfileName)
}

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{
		{RelativePath: "auth.json", Required: true, Sensitive: true},
		{RelativePath: "config.toml", Required: false, Sensitive: false},
	}
}
