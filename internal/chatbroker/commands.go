package chatbroker

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func (b *Broker) handleCommand(ctx context.Context, chatID modeluuid.UUID, thread *Thread, name string, args []string) (string, error) {
	switch name {
	case "new":
		workspace := ""
		if len(args) > 0 {
			workspace = args[0]
		}
		conv, err := b.StartSession(ctx, chatID, thread, workspace, true)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("conversation started\ncontainer: %s\nworkspace: %s", conv.ContainerName, conv.WorkspaceHost), nil
	case "refresh":
		conv, err := b.GetActiveSession(ctx, thread)
		if err != nil {
			return "", err
		}
		if conv == nil {
			return "no active conversation", nil
		}
		if err := b.RefreshSession(ctx, conv); err != nil {
			return "", err
		}
		return "conversation runtime refreshed", nil
	case "purge":
		conv, err := b.GetActiveSession(ctx, thread)
		if err != nil {
			return "", err
		}
		if conv == nil {
			return "no active conversation", nil
		}
		if err := b.PurgeSession(ctx, conv); err != nil {
			return "", err
		}
		return "conversation purged", nil
	case "stop":
		conv, err := b.GetActiveSession(ctx, thread)
		if err != nil {
			return "", err
		}
		if conv == nil {
			return "no active conversation", nil
		}
		if err := b.StopSession(ctx, conv); err != nil {
			return "", err
		}
		return "conversation stopped", nil
	case "status":
		conv, err := b.GetActiveSession(ctx, thread)
		if err != nil {
			return "", err
		}
		if conv == nil {
			return "no active conversation", nil
		}
		msg := fmt.Sprintf(
			"active conversation\ncontainer: %s\nworkspace: %s\ninitialized: %t",
			conv.ContainerName,
			conv.WorkspaceHost,
			conv.Initialized,
		)
		if conv.LastError != "" {
			msg += "\nlast_error: " + conv.LastError
		}
		return msg, nil
	case "upgrade":
		if b.Config == nil || !b.Config.ChatProcessToolsEnabledByID(chatID) {
			return "upgrade is not enabled for this chat", nil
		}
		if b.ProcessActions == nil {
			return "upgrade is not available in this runtime", nil
		}
		if err := b.ProcessActions.Upgrade(ctx); err != nil {
			return "", err
		}
		return "upgrade completed\ntype /quit to restart", nil
	case "quit":
		if b.Config == nil || !b.Config.ChatProcessToolsEnabledByID(chatID) {
			return "quit is not enabled for this chat", nil
		}
		if b.ProcessActions == nil {
			return "quit is not available in this runtime", nil
		}
		if err := b.ProcessActions.Quit(ctx); err != nil {
			return "", err
		}
		return "shutting down ctgbot", nil
	case "help":
		return helpText, nil
	default:
		return "", fmt.Errorf("unknown command %q", name)
	}
}
