package appstate

import (
	"os"
	"path/filepath"
)

type ChatProfileConfig struct {
	chat ChatConfig
}

func (c ChatConfig) Profile() ChatProfileConfig {
	return ChatProfileConfig{chat: c}
}

func (p ChatProfileConfig) Root() string {
	chat := p.chat
	if chat.cfg == nil || chat.cfg.root == "" || chat.chatID.IsNull() {
		return ""
	}
	return filepath.Join(filepath.Dir(chat.cfg.root), "chats", chat.chatID.String())
}

func (p ChatProfileConfig) RuntimeName() string {
	if p.chat.chatID.IsNull() {
		return ""
	}
	return p.chat.chatID.String()
}

func (p ChatProfileConfig) CodexProfileDir() string {
	return filepath.Join(p.Root(), ".codex")
}

func (p ChatProfileConfig) WorkspaceDir() string {
	return filepath.Join(p.Root(), "workspace")
}

func (p ChatProfileConfig) LogDir() string {
	return filepath.Join(p.Root(), "logs")
}

func (p ChatProfileConfig) TLSDir() string {
	return filepath.Join(p.Root(), "tls")
}

func (p ChatProfileConfig) ThreadsRoot() string {
	return filepath.Join(p.Root(), "threads")
}

func (p ChatProfileConfig) EnsurePaths() error {
	for _, dir := range []string{
		p.Root(),
		p.CodexProfileDir(),
		p.WorkspaceDir(),
		p.LogDir(),
		p.TLSDir(),
		p.ThreadsRoot(),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return ensureGitWorkspace(p.WorkspaceDir())
}
