package gmailv2

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

func (c *Component) profileName() string {
	if c == nil {
		return Type
	}
	name := strings.TrimSpace(c.registration.Name)
	if name == "" {
		name = Type
	}
	return safePathSegment(name)
}

func safePathSegment(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "mailbox"
	}
	return b.String()
}

func (c *Component) runtimeMessagePath(messageID string, ext string) string {
	return runtimeMailboxPath(c.profileName(), "messages", strings.TrimSpace(messageID)+ext)
}

func (c *Component) runtimeAttachmentPath(attachmentID string) string {
	return runtimeMailboxPath(c.profileName(), "attachments", strings.TrimSpace(attachmentID)+".bin")
}

func runtimeMailboxPath(profile string, parts ...string) string {
	all := []string{runtimepkg.DefaultWorkspaceRuntimePath, Type, profile}
	all = append(all, parts...)
	return filepath.ToSlash(filepath.Join(all...))
}

func (c *Component) hostMessagePath(workspace string, messageID string, ext string) string {
	return filepath.Join(workspace, Type, c.profileName(), "messages", strings.TrimSpace(messageID)+ext)
}

func (c *Component) hostAttachmentPath(workspace string, attachmentID string) string {
	return filepath.Join(workspace, Type, c.profileName(), "attachments", strings.TrimSpace(attachmentID)+".bin")
}

func (c *Component) workspacePaths(ctx context.Context) ([]string, error) {
	if c == nil || c.storage == nil || c.resolveChatWorkspace == nil {
		return nil, nil
	}
	binding, err := c.storage.ChatComponents().FindByComponentRoleAndExternalChannelID(ctx, c.componentID, coremodel.ChatComponentRoleSource, c.providerChannelID())
	if err != nil || binding == nil {
		return nil, err
	}
	chat, err := c.storage.Chats().GetByID(ctx, binding.ChatID)
	if err != nil || chat == nil {
		return nil, err
	}
	workspace, err := c.resolveChatWorkspace(ctx, *chat)
	if err != nil {
		return nil, fmt.Errorf("resolve gmailv2 workspace: %w", err)
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, nil
	}
	return []string{workspace}, nil
}
