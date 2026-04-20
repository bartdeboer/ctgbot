package chatbroker

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/configcommands"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func (b *Broker) handleCommand(ctx context.Context, chatID modeluuid.UUID, thread *Thread, userID int64, isAdmin bool, name string, args []string) (string, error) {
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
		return fmt.Sprintf("conversation started\ncontainer: %s\nworkspace: %s", conv.ContainerName(b.Config), conv.WorkspaceHost), nil
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
	case "interrupt":
		conv, err := b.GetActiveSession(ctx, thread)
		if err != nil {
			return "", err
		}
		if conv == nil {
			return "no active conversation", nil
		}
		if b.Config == nil || !b.Config.ChatInteractiveInterruptEnabledByID(chatID) {
			return "interrupt is disabled for this chat", nil
		}
		if !b.interruptThread(conv.ID, b.sandboxForThread(conv)) {
			return "no active run to interrupt", nil
		}
		return "interrupt requested", nil
	case "config":
		return b.handleConfigCommand(chatID, userID, isAdmin, args)
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
			conv.ContainerName(b.Config),
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

func (b *Broker) handleConfigCommand(chatID modeluuid.UUID, userID int64, isAdmin bool, args []string) (string, error) {
	if b == nil || b.ConfigCommands == nil {
		return "", fmt.Errorf("config commands are unavailable")
	}
	pctx := configcommands.ContextForChat(b.Config, chatID, userID, isAdmin)
	if len(args) == 0 {
		return "usage:\n/config list\n/config set <key> <value>", nil
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "list":
		if len(args) != 1 {
			return "", fmt.Errorf("usage: /config list")
		}
		return b.ConfigCommands.List(pctx)
	case "set":
		if len(args) < 3 {
			return "", fmt.Errorf("usage: /config set <key> <value>")
		}
		return b.ConfigCommands.Set(pctx, args[1], strings.Join(args[2:], " "))
	default:
		return "", fmt.Errorf("usage: /config list or /config set <key> <value>")
	}
}
