package appstate

import (
	"context"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type GitIdentity struct {
	Name  string
	Email string
}

type GitConfig struct {
	cfg *Config
}

type gitConfigValue struct {
	UserName  string `json:"user_name"`
	UserEmail string `json:"user_email"`
}

func (c *Config) Git() GitConfig {
	return GitConfig{cfg: c}
}

func (i GitIdentity) Complete() bool {
	return strings.TrimSpace(i.Name) != "" && strings.TrimSpace(i.Email) != ""
}

func (i GitIdentity) Env() []string {
	name := strings.TrimSpace(i.Name)
	email := strings.TrimSpace(i.Email)
	if name == "" || email == "" {
		return nil
	}
	return []string{
		"GIT_AUTHOR_NAME=" + name,
		"GIT_AUTHOR_EMAIL=" + email,
		"GIT_COMMITTER_NAME=" + name,
		"GIT_COMMITTER_EMAIL=" + email,
	}
}

func (g GitConfig) ExplicitIdentity() (GitIdentity, bool) {
	if g.cfg == nil {
		return GitIdentity{}, false
	}
	var grouped gitConfigValue
	hasGrouped := g.cfg.structValue("git", &grouped)
	name := g.cfg.string("git.user_name", "")
	email := g.cfg.string("git.user_email", "")
	if name == "" {
		name = strings.TrimSpace(grouped.UserName)
	}
	if email == "" {
		email = strings.TrimSpace(grouped.UserEmail)
	}
	configured := name != "" || email != "" || (hasGrouped && (strings.TrimSpace(grouped.UserName) != "" || strings.TrimSpace(grouped.UserEmail) != ""))
	return GitIdentity{Name: name, Email: email}, configured
}

func (c *Config) ResolveGitIdentity(ctx context.Context) GitIdentity {
	if explicit, configured := c.Git().ExplicitIdentity(); configured {
		if explicit.Complete() {
			return explicit
		}
		return GitIdentity{}
	}
	if host := c.HostGitIdentity(ctx); host.Complete() {
		return host
	}
	return GitIdentity{}
}

func (c *Config) ParseThreadContainerName(name string) (modeluuid.UUID, bool) {
	raw := strings.TrimPrefix(strings.TrimSpace(name), "ctgbot-")
	if raw == "" || raw == name {
		return modeluuid.Nil, false
	}
	threadID, err := modeluuid.Parse(raw)
	if err != nil {
		return modeluuid.Nil, false
	}
	return threadID, true
}

func (c *Config) ParseChatClientIdentity(name string) (modeluuid.UUID, bool) {
	raw := strings.TrimPrefix(strings.TrimSpace(name), "ctgbot-chat-")
	if raw == "" || raw == name {
		return modeluuid.Nil, false
	}
	chatID, err := modeluuid.Parse(raw)
	if err != nil {
		return modeluuid.Nil, false
	}
	return chatID, true
}

func (c *Config) HostGitIdentity(ctx context.Context) GitIdentity {
	return GitIdentity{
		Name:  strings.TrimSpace(readGlobalGitConfig(ctx, "user.name")),
		Email: strings.TrimSpace(readGlobalGitConfig(ctx, "user.email")),
	}
}
