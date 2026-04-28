// Package codex contains the component-model Codex implementation skeleton.
//
// This package is intentionally separate from internal/agent/codexengine. The
// existing agent path can keep serving the current broker while a future broker
// can build on component/codex directly.
package codex

import "github.com/bartdeboer/ctgbot/internal/component"

const ComponentType = "codex"

type Component struct{}

var _ component.Component = (*Component)(nil)
var _ component.ProfileOwner = (*Component)(nil)

func New() *Component {
	return &Component{}
}

func (c *Component) Type() string {
	return ComponentType
}

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{
		{RelativePath: "auth.json", Required: true, Sensitive: true},
		{RelativePath: "config.toml", Required: false, Sensitive: false},
	}
}
