package outlook

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
)

const (
	outlookConfigTenant          = "tenant"
	outlookConfigClientID        = "client-id"
	outlookConfigUserID          = "user-id"
	outlookConfigMailboxEmail    = "mailbox-email"
	outlookConfigPollFolder      = "poll-folder"
	outlookConfigPollInterval    = "poll-interval"
	outlookConfigMaxPollMessages = "max-poll-messages"
)

func (c *Component) ConfigSchema(ctx context.Context, req commandengine.Request) (configsurface.ConfigSchema, error) {
	_, _ = ctx, req
	d := defaultComponentConfig()
	return configsurface.ConfigSchema{Fields: []configsurface.FieldSchema{
		{Key: outlookConfigTenant, Help: "Microsoft tenant: organizations, common, or tenant id", Type: configsurface.FieldTypeString, Writable: true, Default: d.Tenant},
		{Key: outlookConfigClientID, Help: "Microsoft app client id", Type: configsurface.FieldTypeString, Writable: true, Secret: true},
		{Key: outlookConfigUserID, Help: "Graph user id", Type: configsurface.FieldTypeString, Writable: true, Default: d.UserID},
		{Key: outlookConfigMailboxEmail, Help: "Expected mailbox email address", Type: configsurface.FieldTypeString, Writable: true},
		{Key: outlookConfigPollFolder, Help: "Mail folder to poll", Type: configsurface.FieldTypeString, Writable: true, Default: d.PollFolder},
		{Key: outlookConfigPollInterval, Help: "Outlook poll interval", Type: configsurface.FieldTypeDuration, Writable: true, Default: d.PollInterval},
		{Key: outlookConfigMaxPollMessages, Help: "Maximum messages to inspect per poll", Type: configsurface.FieldTypeInt, Writable: true, Default: strconv.Itoa(d.MaxPollMessages)},
	}}, nil
}

func (c *Component) ConfigGet(ctx context.Context, req commandengine.Request, key string) (string, error) {
	_, _ = ctx, req
	cfg := c.componentConfig.withDefaults()
	switch configsurface.NormalizeKey(key) {
	case outlookConfigTenant:
		return cfg.Tenant, nil
	case outlookConfigClientID:
		if cfg.ClientID != "" {
			return "<configured>", nil
		}
		return "", nil
	case outlookConfigUserID:
		return cfg.UserID, nil
	case outlookConfigMailboxEmail:
		return cfg.MailboxEmail, nil
	case outlookConfigPollFolder:
		return cfg.PollFolder, nil
	case outlookConfigPollInterval:
		return cfg.pollInterval().String(), nil
	case outlookConfigMaxPollMessages:
		return strconv.Itoa(cfg.MaxPollMessages), nil
	default:
		return "", fmt.Errorf("unknown outlook config %q", key)
	}
}

func (c *Component) ConfigSet(ctx context.Context, req commandengine.Request, key string, value string) error {
	_, _ = ctx, req
	cfg := c.componentConfig.withDefaults()
	key = configsurface.NormalizeKey(key)
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("missing config value")
	}
	switch key {
	case outlookConfigTenant:
		cfg.Tenant = value
	case outlookConfigClientID:
		cfg.ClientID = value
	case outlookConfigUserID:
		cfg.UserID = value
	case outlookConfigMailboxEmail:
		cfg.MailboxEmail = value
	case outlookConfigPollFolder:
		cfg.PollFolder = value
	case outlookConfigPollInterval:
		if _, err := time.ParseDuration(value); err != nil {
			return err
		}
		cfg.PollInterval = value
	case outlookConfigMaxPollMessages:
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return fmt.Errorf("config %s expects positive integer", key)
		}
		cfg.MaxPollMessages = n
	default:
		return fmt.Errorf("unknown outlook config %q", key)
	}
	if err := saveComponentConfig(c.profile.Path, cfg); err != nil {
		return err
	}
	c.componentConfig = cfg.withDefaults()
	return nil
}

func (c *Component) ConfigUnset(ctx context.Context, req commandengine.Request, key string) error {
	_, _ = ctx, req
	cfg := c.componentConfig.withDefaults()
	switch configsurface.NormalizeKey(key) {
	case outlookConfigTenant:
		cfg.Tenant = ""
	case outlookConfigClientID:
		cfg.ClientID = ""
	case outlookConfigUserID:
		cfg.UserID = ""
	case outlookConfigMailboxEmail:
		cfg.MailboxEmail = ""
	case outlookConfigPollFolder:
		cfg.PollFolder = ""
	case outlookConfigPollInterval:
		cfg.PollInterval = ""
	case outlookConfigMaxPollMessages:
		cfg.MaxPollMessages = 0
	default:
		return fmt.Errorf("unknown outlook config %q", key)
	}
	if err := saveComponentConfig(c.profile.Path, cfg); err != nil {
		return err
	}
	c.componentConfig = cfg.withDefaults()
	return nil
}
