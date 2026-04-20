package appstate

import (
	"strconv"
	"strings"
	"time"
)

func (c *Config) TelegramAdminUserID() int64 {
	if c == nil || c.Store == nil {
		return 0
	}
	raw := strings.TrimSpace(c.Store.GetString("telegram.admin_user_id", ""))
	if raw == "" {
		return 0
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func (c *Config) TelegramPollTimeout() time.Duration {
	return c.durationFromConfig("telegram.defaults.poll_timeout_sec", 60, time.Second)
}

func (c *Config) TelegramDebounceWindow() time.Duration {
	d := c.durationFromConfig("telegram.defaults.debounce_ms", 800, time.Millisecond)
	if d <= 0 {
		return 0
	}
	return d
}

func (c *Config) TelegramRenderFormat() string {
	format := "plain"
	if c != nil && c.Store != nil {
		format = strings.TrimSpace(strings.ToLower(c.Store.GetString("telegram.defaults.render_format", format)))
	}
	switch format {
	case "text", "plain", "":
		return "plain"
	case "html":
		return "html"
	case "markdown", "markdownv2", "markdown_v2":
		return "markdown_v2"
	default:
		return "plain"
	}
}
