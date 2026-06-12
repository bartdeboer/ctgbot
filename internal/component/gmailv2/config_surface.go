package gmailv2

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configengine"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
)

const (
	gmailv2ConfigUserID           = "user-id"
	gmailv2ConfigMailboxEmail     = "mailbox-email"
	gmailv2ConfigPollInterval     = "poll-interval"
	gmailv2ConfigMaxPollMessages  = "max-poll-messages"
	gmailv2ConfigDefaultShowFull  = "default-show-full"
	gmailv2ConfigMaterializeRaw   = "materialize-raw"
	gmailv2ConfigMaterializeHTML  = "materialize-html"
	gmailv2ConfigSkipLabels       = "skip-labels"
	gmailv2ConfigIncludeSpamTrash = "include-spam-trash"
)

func (c *Component) ConfigSchema(ctx context.Context, req commandengine.Request) (configsurface.ConfigSchema, error) {
	_, _ = ctx, req
	defaults := defaultComponentConfig().withDefaults()
	return configsurface.ConfigSchema{Fields: []configsurface.FieldSchema{
		{Key: gmailv2ConfigUserID, Help: "Gmail API user id", Type: configsurface.FieldTypeString, Writable: true, Default: defaults.UserID},
		{Key: gmailv2ConfigMailboxEmail, Help: "Expected mailbox email address", Type: configsurface.FieldTypeString, Writable: true},
		{Key: gmailv2ConfigPollInterval, Help: "Gmail poll interval", Type: configsurface.FieldTypeDuration, Writable: true, Default: defaults.PollInterval},
		{Key: gmailv2ConfigMaxPollMessages, Help: "Maximum messages to emit per poll", Type: configsurface.FieldTypeInt, Writable: true, Default: strconv.Itoa(defaults.MaxPollMessages)},
		{Key: gmailv2ConfigDefaultShowFull, Help: "Show full bodies for unknown senders by default", Type: configsurface.FieldTypeBool, Writable: true, Default: strconv.FormatBool(defaults.DefaultShowFull), Options: []string{"true", "false"}},
		{Key: gmailv2ConfigMaterializeRaw, Help: "Write raw .eml files for stored messages", Type: configsurface.FieldTypeBool, Writable: true, Default: strconv.FormatBool(defaults.materializeRaw()), Options: []string{"true", "false"}},
		{Key: gmailv2ConfigMaterializeHTML, Help: "Write HTML files for stored messages when available", Type: configsurface.FieldTypeBool, Writable: true, Default: strconv.FormatBool(defaults.materializeHTML()), Options: []string{"true", "false"}},
		{Key: gmailv2ConfigSkipLabels, Help: "Comma-separated Gmail label IDs skipped during polling", Type: configsurface.FieldTypeStringList, Writable: true, Default: strings.Join(defaults.SkipLabels, ",")},
		{Key: gmailv2ConfigIncludeSpamTrash, Help: "Include spam/trash when default skip labels are computed", Type: configsurface.FieldTypeBool, Writable: true, Default: strconv.FormatBool(defaults.IncludeSpamTrash), Options: []string{"true", "false"}},
	}}, nil
}

func (c *Component) ConfigGet(ctx context.Context, req commandengine.Request, key string) (string, error) {
	_, _ = ctx, req
	config := c.currentConfig()
	switch configsurface.NormalizeKey(key) {
	case gmailv2ConfigUserID:
		return config.UserID, nil
	case gmailv2ConfigMailboxEmail:
		return config.MailboxEmail, nil
	case gmailv2ConfigPollInterval:
		return config.pollInterval().String(), nil
	case gmailv2ConfigMaxPollMessages:
		return strconv.Itoa(config.MaxPollMessages), nil
	case gmailv2ConfigDefaultShowFull:
		return strconv.FormatBool(config.DefaultShowFull), nil
	case gmailv2ConfigMaterializeRaw:
		return strconv.FormatBool(config.materializeRaw()), nil
	case gmailv2ConfigMaterializeHTML:
		return strconv.FormatBool(config.materializeHTML()), nil
	case gmailv2ConfigSkipLabels:
		return strings.Join(config.SkipLabels, ","), nil
	case gmailv2ConfigIncludeSpamTrash:
		return strconv.FormatBool(config.IncludeSpamTrash), nil
	default:
		return "", unknownGmailV2Config(key)
	}
}

func (c *Component) ConfigSet(ctx context.Context, req commandengine.Request, key string, value string) error {
	_, _ = ctx, req
	config := c.currentConfig()
	key = configsurface.NormalizeKey(key)
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("missing config value")
	}
	switch key {
	case gmailv2ConfigUserID:
		config.UserID = value
	case gmailv2ConfigMailboxEmail:
		config.MailboxEmail = value
	case gmailv2ConfigPollInterval:
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("config %s expects duration: %w", key, err)
		}
		config.PollInterval = value
	case gmailv2ConfigMaxPollMessages:
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("config %s expects positive integer", key)
		}
		config.MaxPollMessages = parsed
	case gmailv2ConfigDefaultShowFull:
		parsed, err := configsurface.ParseBool(value)
		if err != nil {
			return fmt.Errorf("config %s expects true or false", key)
		}
		config.DefaultShowFull = parsed
	case gmailv2ConfigMaterializeRaw:
		parsed, err := configsurface.ParseBool(value)
		if err != nil {
			return fmt.Errorf("config %s expects true or false", key)
		}
		config.MaterializeRaw = &parsed
	case gmailv2ConfigMaterializeHTML:
		parsed, err := configsurface.ParseBool(value)
		if err != nil {
			return fmt.Errorf("config %s expects true or false", key)
		}
		config.MaterializeHTML = &parsed
	case gmailv2ConfigSkipLabels:
		config.SkipLabels = configengine.String(value).StringList()
	case gmailv2ConfigIncludeSpamTrash:
		parsed, err := configsurface.ParseBool(value)
		if err != nil {
			return fmt.Errorf("config %s expects true or false", key)
		}
		config.IncludeSpamTrash = parsed
	default:
		return unknownGmailV2Config(key)
	}
	return c.saveConfig(config)
}

func (c *Component) ConfigUnset(ctx context.Context, req commandengine.Request, key string) error {
	_, _ = ctx, req
	config := c.currentConfig()
	switch configsurface.NormalizeKey(key) {
	case gmailv2ConfigUserID:
		config.UserID = ""
	case gmailv2ConfigMailboxEmail:
		config.MailboxEmail = ""
	case gmailv2ConfigPollInterval:
		config.PollInterval = ""
	case gmailv2ConfigMaxPollMessages:
		config.MaxPollMessages = 0
	case gmailv2ConfigDefaultShowFull:
		config.DefaultShowFull = false
	case gmailv2ConfigMaterializeRaw:
		config.MaterializeRaw = nil
	case gmailv2ConfigMaterializeHTML:
		config.MaterializeHTML = nil
	case gmailv2ConfigSkipLabels:
		config.SkipLabels = nil
	case gmailv2ConfigIncludeSpamTrash:
		config.IncludeSpamTrash = false
	default:
		return unknownGmailV2Config(key)
	}
	return c.saveConfig(config)
}

func (c *Component) currentConfig() ComponentConfig {
	if c == nil {
		return defaultComponentConfig().withDefaults()
	}
	return c.componentConfig.withDefaults()
}

func (c *Component) saveConfig(config ComponentConfig) error {
	if c == nil {
		return fmt.Errorf("missing gmailv2 component")
	}
	config = config.withDefaults()
	if err := saveComponentConfig(c.profile.Path, config); err != nil {
		return err
	}
	c.componentConfig = config
	c.UserID = config.UserID
	return nil
}

func unknownGmailV2Config(key string) error {
	return fmt.Errorf("unknown gmailv2 config %q", configsurface.NormalizeKey(key))
}
