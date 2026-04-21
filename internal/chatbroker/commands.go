package chatbroker

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/chatcommands"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func (b *Broker) handleCommand(ctx context.Context, chatID modeluuid.UUID, thread *Thread, userID int64, isAdmin bool, name string, args []string) (string, error) {
	if reply, handled, err := b.handleSharedChatCommand(ctx, thread, isAdmin, append([]string{name}, args...)); handled {
		return reply, err
	}

	switch name {
	case "new":
		return "use /container refresh to rebuild the backing container, or /chat purge to drop the active chat state", nil
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
			conv.ContainerName(b.Config),
			conv.WorkspaceHost,
			conv.Initialized,
		)
		if conv.LastError != "" {
			msg += "\nlast_error: " + conv.LastError
		}
		return msg, nil
	case "help":
		return chatcommands.New(nil).UserHelpText(), nil
	default:
		return "", fmt.Errorf("unknown command %q", name)
	}
}

func (b *Broker) handleSharedChatCommand(ctx context.Context, thread *Thread, isAdmin bool, argv []string) (string, bool, error) {
	if !shouldUseSharedChatCommands(argv) {
		return "", false, nil
	}
	provider := NewChatCommandsProvider(b)
	cmds := chatcommands.New(chatcommands.NewProviderRunner(provider))
	result, err := cmds.RunUserRequest(ctx, chatcommands.Request{
		ThreadID: threadIDOrNil(thread),
		Context:  chatcommands.CommandContext{IsRoot: isAdmin},
	}, argv)
	if err != nil {
		return "", true, err
	}
	if result.Session != nil {
		return fmt.Sprintf("conversation started\ncontainer: %s\nworkspace: %s", result.Session.Container, result.Session.Workspace), true, nil
	}
	return strings.TrimSpace(result.Text), true, nil
}

func shouldUseSharedChatCommands(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(argv[0])) {
	case "config", "refresh", "purge", "interrupt", "upgrade", "quit", "container", "chat":
		return true
	default:
		return false
	}
}
