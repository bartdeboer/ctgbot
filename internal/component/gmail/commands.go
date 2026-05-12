package gmail

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

var _ component.CommandSurface = (*Component)(nil)
var _ component.LocalCommandSurface = (*Component)(nil)

type statusCommand struct{}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{{
		Pattern: "status",
		Help:    "Show Gmail component status",
		Build: func(req *clir.Request) (any, error) {
			_ = req
			return statusCommand{}, nil
		},
		Sources: []commandengine.Source{commandengine.SourceCLI},
		Policy:  simplerbac.Any(simplerbac.RoleRoot),
	}}
}

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	return commandengine.RegisterPattern[statusCommand](registry, "status", func(ctx context.Context, req commandengine.Request, cmd statusCommand) (commandengine.Result, error) {
		_, _ = req, cmd
		return c.status(ctx)
	})
}

func (c *Component) status(ctx context.Context) (commandengine.Result, error) {
	if c == nil {
		return commandengine.Result{}, fmt.Errorf("missing gmail component")
	}
	var lines []string
	lines = append(lines, "gmail status")
	lines = append(lines, "component: "+c.registration.Ref())
	lines = append(lines, "home: "+strings.TrimSpace(c.home.Path))
	lines = append(lines, "poll_interval: "+c.componentConfig.pollInterval().String())
	account, authState := c.authSummary(ctx)
	lines = append(lines, "auth: "+authState)
	if account != "" {
		lines = append(lines, "account: "+account)
	}
	state, err := c.loadState()
	if err == nil && state.HistoryID != 0 {
		lines = append(lines, fmt.Sprintf("history_id: %d", state.HistoryID))
		if !state.LastSyncedAt.IsZero() {
			lines = append(lines, "last_synced_at: "+state.LastSyncedAt.Format("2006-01-02T15:04:05Z07:00"))
		}
		if state.LastMessageID != "" {
			lines = append(lines, "last_message_id: "+state.LastMessageID)
		}
	}
	if defaultID, err := c.DefaultSourceExternalChannelID(ctx); err == nil && strings.TrimSpace(defaultID) != "" {
		lines = append(lines, "default_external_channel_id: "+defaultID)
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (c *Component) authSummary(ctx context.Context) (string, string) {
	service, err := c.serviceFromStoredToken(ctx)
	if err != nil {
		return "", "not authenticated"
	}
	profile, err := service.Users.GetProfile(c.userID()).Context(ctx).Do()
	if err != nil {
		return "", "token unavailable"
	}
	account := strings.TrimSpace(profile.EmailAddress)
	c.mailboxEmail = account
	return account, "authenticated"
}

func (c *Component) DefaultSourceExternalChannelID(ctx context.Context) (string, error) {
	if c == nil {
		return "", fmt.Errorf("missing gmail component")
	}
	if value, ok := c.localProviderChannelID(); ok {
		return value, nil
	}
	service, err := c.serviceFromStoredToken(ctx)
	if err != nil {
		return "", fmt.Errorf("gmail source binding needs auth or component.json mailbox_email: %w", err)
	}
	profile, err := service.Users.GetProfile(c.userID()).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("gmail source binding needs auth or component.json mailbox_email: get gmail profile: %w", err)
	}
	c.mailboxEmail = strings.TrimSpace(profile.EmailAddress)
	if c.mailboxEmail == "" {
		if value, ok := c.localProviderChannelID(); ok {
			return value, nil
		}
		return "", fmt.Errorf("gmail source binding needs auth or component.json mailbox_email")
	}
	state, _ := c.loadState()
	state.MailboxEmail = c.mailboxEmail
	_ = c.saveState(state)
	return c.mailboxEmail, nil
}
