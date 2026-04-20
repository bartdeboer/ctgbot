package appstate

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func (c *Config) DBPath() string {
	return filepath.Join(c.Root(), "ctgbot.db")
}

func (c *Config) ChatsRoot() string {
	return filepath.Join(c.ProjectRoot(), "chats")
}

func (c *Config) ChatRuntimeName(chatID modeluuid.UUID) string {
	return chatID.String()
}

func (c *Config) ThreadContainerName(threadID modeluuid.UUID) string {
	if threadID.IsNull() {
		return ""
	}
	return fmt.Sprintf("%s%s", namePrefix, threadID.String())
}

func (c *Config) ChatClientIdentity(chatID modeluuid.UUID) string {
	if chatID.IsNull() {
		return ""
	}
	return fmt.Sprintf("%s%s", chatClientNamePrefix, chatID.String())
}

func (c *Config) ParseThreadContainerName(name string) (threadID modeluuid.UUID, ok bool) {
	raw := strings.TrimPrefix(strings.TrimSpace(name), namePrefix)
	if raw == "" || raw == name {
		return modeluuid.Nil, false
	}
	threadID, err := modeluuid.Parse(raw)
	if err != nil {
		return modeluuid.Nil, false
	}
	return threadID, true
}

func (c *Config) ParseChatClientIdentity(name string) (chatID modeluuid.UUID, ok bool) {
	raw := strings.TrimPrefix(strings.TrimSpace(name), chatClientNamePrefix)
	if raw == "" || raw == name {
		return modeluuid.Nil, false
	}
	chatID, err := modeluuid.Parse(raw)
	if err != nil {
		return modeluuid.Nil, false
	}
	return chatID, true
}

func (c *Config) ChatRoot(name string) string {
	return filepath.Join(c.ChatsRoot(), name)
}

func (c *Config) ChatRuntimeRoot(chatID modeluuid.UUID) string {
	return c.ChatRoot(c.ChatRuntimeName(chatID))
}

func (c *Config) DefaultChatCodexProfileDirByID(chatID modeluuid.UUID) string {
	return filepath.Join(c.ChatRuntimeRoot(chatID), ".codex")
}

func (c *Config) DefaultChatWorkspaceDirByID(chatID modeluuid.UUID) string {
	return filepath.Join(c.ChatRuntimeRoot(chatID), "workspace")
}

func (c *Config) DefaultChatLogDirByID(chatID modeluuid.UUID) string {
	return filepath.Join(c.ChatRuntimeRoot(chatID), "logs")
}

func (c *Config) DefaultChatTLSDirByID(chatID modeluuid.UUID) string {
	return filepath.Join(c.ChatRuntimeRoot(chatID), "tls")
}

func (c *Config) ChatThreadsRoot(chatID modeluuid.UUID) string {
	return filepath.Join(c.ChatRuntimeRoot(chatID), "threads")
}

func (c *Config) HostGitIdentity(ctx context.Context) GitIdentity {
	return GitIdentity{
		Name:  strings.TrimSpace(readGitConfig(ctx, "user.name")),
		Email: strings.TrimSpace(readGitConfig(ctx, "user.email")),
	}
}
