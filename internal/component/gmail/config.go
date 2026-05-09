package gmail

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

	DefaultPollInterval = time.Minute
)

type ComponentConfig struct {
	UserID           string   `json:"user_id"`
	MailboxEmail     string   `json:"mailbox_email,omitempty"`
	PollInterval     string   `json:"poll_interval"`
	SkipLabels       []string `json:"skip_labels,omitempty"`
	IncludeSpamTrash bool     `json:"include_spam_trash,omitempty"`
}

func loadComponentConfig(homePath string) (ComponentConfig, error) {
	path := filepath.Join(strings.TrimSpace(homePath), ComponentConfigFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ComponentConfig{}.withDefaults(), nil
		}
		return ComponentConfig{}, fmt.Errorf("read gmail component config %s: %w", path, err)
	}
	var config ComponentConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return ComponentConfig{}, fmt.Errorf("read gmail component config %s: %w", path, err)
	}
	return config.withDefaults(), nil
}

func (c ComponentConfig) withDefaults() ComponentConfig {
	c.UserID = strings.TrimSpace(c.UserID)
	if c.UserID == "" {
		c.UserID = DefaultUserID
	}
	c.MailboxEmail = strings.TrimSpace(c.MailboxEmail)
	c.PollInterval = strings.TrimSpace(c.PollInterval)
	if c.PollInterval == "" {
		c.PollInterval = DefaultPollInterval.String()
	}
	c.SkipLabels = cleanLabelIDs(c.SkipLabels)
	if len(c.SkipLabels) == 0 {
		c.SkipLabels = []string{"SENT", "DRAFT"}
		if !c.IncludeSpamTrash {
			c.SkipLabels = append(c.SkipLabels, "SPAM", "TRASH")
		}
	}
	return c
}

func (c ComponentConfig) pollInterval() time.Duration {
	duration, err := time.ParseDuration(strings.TrimSpace(c.PollInterval))
	if err != nil || duration <= 0 {
		return DefaultPollInterval
	}
	return duration
}

func cleanLabelIDs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
