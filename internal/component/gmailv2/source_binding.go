package gmailv2

import (
	"context"
	"fmt"
	"strings"
)

func (c *Component) DefaultSourceExternalChannelID(ctx context.Context) (string, error) {
	if c == nil {
		return "", fmt.Errorf("missing gmailv2 component")
	}
	if value, ok := c.localProviderChannelID(); ok {
		return value, nil
	}
	service, err := c.serviceFromStoredToken(ctx)
	if err != nil {
		return "", fmt.Errorf("gmailv2 source binding needs auth or component.json mailbox_email: %w", err)
	}
	profile, err := service.Users.GetProfile(c.userID()).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("gmailv2 source binding needs auth or component.json mailbox_email: get gmail profile: %w", err)
	}
	c.mailboxEmail = strings.TrimSpace(profile.EmailAddress)
	if c.mailboxEmail == "" {
		return "", fmt.Errorf("gmailv2 source binding needs auth or component.json mailbox_email")
	}
	state, _ := c.loadState()
	state.MailboxEmail = c.mailboxEmail
	_ = c.saveState(state)
	return c.mailboxEmail, nil
}
