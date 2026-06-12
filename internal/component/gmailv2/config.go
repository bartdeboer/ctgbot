package gmailv2

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

	DefaultPollInterval    = time.Minute
	DefaultMaxPollMessages = 20
)

type ComponentConfig struct {
	UserID           string   `json:"user_id"`
	MailboxEmail     string   `json:"mailbox_email,omitempty"`
	PollInterval     string   `json:"poll_interval"`
	MaxPollMessages  int      `json:"max_poll_messages,omitempty"`
	DefaultShowFull  bool     `json:"default_show_full,omitempty"`
	MaterializeRaw   *bool    `json:"materialize_raw,omitempty"`
	MaterializeHTML  *bool    `json:"materialize_html,omitempty"`
	SkipLabels       []string `json:"skip_labels,omitempty"`
	IncludeSpamTrash bool     `json:"include_spam_trash,omitempty"`
}

func loadComponentConfig(profilePath string) (ComponentConfig, error) {
	path := filepath.Join(strings.TrimSpace(profilePath), ComponentConfigFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultComponentConfig().withDefaults(), nil
		}
		return ComponentConfig{}, fmt.Errorf("read gmailv2 component config %s: %w", path, err)
	}
	config := defaultComponentConfig()
	if err := json.Unmarshal(data, &config); err != nil {
		return ComponentConfig{}, fmt.Errorf("read gmailv2 component config %s: %w", path, err)
	}
	return config.withDefaults(), nil
}

func saveComponentConfig(profilePath string, config ComponentConfig) error {
	path := filepath.Join(strings.TrimSpace(profilePath), ComponentConfigFilename)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create gmailv2 component config dir: %w", err)
	}
	data, err := json.MarshalIndent(config.withDefaults(), "", "  ")
	if err != nil {
		return fmt.Errorf("encode gmailv2 component config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write gmailv2 component config %s: %w", path, err)
	}
	return nil
}

func defaultComponentConfig() ComponentConfig {
	yes := true
	return ComponentConfig{
		UserID:          DefaultUserID,
		PollInterval:    DefaultPollInterval.String(),
		MaxPollMessages: DefaultMaxPollMessages,
		MaterializeRaw:  &yes,
		MaterializeHTML: &yes,
	}
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
	if c.MaxPollMessages <= 0 {
		c.MaxPollMessages = DefaultMaxPollMessages
	}
	if c.MaterializeRaw == nil {
		yes := true
		c.MaterializeRaw = &yes
	}
	if c.MaterializeHTML == nil {
		yes := true
		c.MaterializeHTML = &yes
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

func (c ComponentConfig) materializeRaw() bool {
	return c.MaterializeRaw == nil || *c.MaterializeRaw
}

func (c ComponentConfig) materializeHTML() bool {
	return c.MaterializeHTML == nil || *c.MaterializeHTML
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
