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
		Name:  strings.TrimSpace(readGitConfig(ctx, "user.name")),
		Email: strings.TrimSpace(readGitConfig(ctx, "user.email")),
	}
}
