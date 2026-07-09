package outlook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	ComponentConfigFilename = "component.json"
	OAuthClientFilename     = "oauth_client.json"
	TokenFilename           = "token.json"
	StateFilename           = "state.json"

	DefaultTenant          = "organizations"
	DefaultUserID          = "me"
	DefaultPollFolder      = "inbox"
	DefaultPollInterval    = time.Minute
	DefaultMaxPollMessages = 20
)

type ComponentConfig struct {
	Tenant          string `json:"tenant,omitempty"`
	ClientID        string `json:"client_id,omitempty"`
	UserID          string `json:"user_id,omitempty"`
	MailboxEmail    string `json:"mailbox_email,omitempty"`
	PollFolder      string `json:"poll_folder,omitempty"`
	PollInterval    string `json:"poll_interval,omitempty"`
	MaxPollMessages int    `json:"max_poll_messages,omitempty"`
}

func loadComponentConfig(profilePath string) (ComponentConfig, error) {
	path := filepath.Join(strings.TrimSpace(profilePath), ComponentConfigFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultComponentConfig(), nil
		}
		return ComponentConfig{}, fmt.Errorf("read outlook component config %s: %w", path, err)
	}
	config := defaultComponentConfig()
	if err := json.Unmarshal(data, &config); err != nil {
		return ComponentConfig{}, fmt.Errorf("parse outlook component config %s: %w", path, err)
	}
	return config.withDefaults(), nil
}

func defaultComponentConfig() ComponentConfig {
	return ComponentConfig{Tenant: DefaultTenant, UserID: DefaultUserID, PollFolder: DefaultPollFolder, PollInterval: DefaultPollInterval.String(), MaxPollMessages: DefaultMaxPollMessages}
}

func (c ComponentConfig) withDefaults() ComponentConfig {
	c.Tenant = strings.TrimSpace(c.Tenant)
	if c.Tenant == "" {
		c.Tenant = DefaultTenant
	}
	c.UserID = strings.TrimSpace(c.UserID)
	if c.UserID == "" {
		c.UserID = DefaultUserID
	}
	c.PollFolder = strings.TrimSpace(c.PollFolder)
	if c.PollFolder == "" {
		c.PollFolder = DefaultPollFolder
	}
	c.PollInterval = strings.TrimSpace(c.PollInterval)
	if c.PollInterval == "" {
		c.PollInterval = DefaultPollInterval.String()
	}
	if c.MaxPollMessages <= 0 {
		c.MaxPollMessages = DefaultMaxPollMessages
	}
	c.MailboxEmail = strings.TrimSpace(c.MailboxEmail)
	c.ClientID = strings.TrimSpace(c.ClientID)
	return c
}

func (c ComponentConfig) pollInterval() time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(c.PollInterval))
	if err != nil || d <= 0 {
		return DefaultPollInterval
	}
	return d
}

func saveComponentConfig(profilePath string, config ComponentConfig) error {
	path := filepath.Join(strings.TrimSpace(profilePath), ComponentConfigFilename)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config.withDefaults(), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
