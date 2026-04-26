package appstate

import "github.com/bartdeboer/ctgbot/internal/modeluuid"

func (c *Config) Thread(chatID modeluuid.UUID, threadID modeluuid.UUID) ThreadConfig {
	return ThreadConfig{cfg: c, chatID: chatID, threadID: threadID}
}

type ThreadConfig struct {
	cfg      *Config
	chatID   modeluuid.UUID
	threadID modeluuid.UUID
}

func (t ThreadConfig) ID() modeluuid.UUID {
	return t.threadID
}

func (t ThreadConfig) Chat() ChatConfig {
	return t.cfg.Chat(t.chatID)
}

func (t ThreadConfig) WorkspaceHostPath() string {
	return t.Chat().WorkspaceHostPath()
}

func (t ThreadConfig) CodexProfileHostPath() string {
	return t.Chat().CodexProfileHostPath()
}

func (t ThreadConfig) ContainerName() string {
	if t.threadID.IsNull() {
		return ""
	}
	return "ctgbot-" + t.threadID.String()
}
