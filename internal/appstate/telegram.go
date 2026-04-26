package appstate

import (
	"strconv"
	"strings"
	"time"
)

func (c *Config) Telegram() TelegramConfig {
	return TelegramConfig{cfg: c}
}

type TelegramConfig struct {
	cfg *Config
}

func (t TelegramConfig) Token() string {
	return t.cfg.string("telegram.token", "")
}

func (t TelegramConfig) SetToken(token string) error {
	return t.cfg.persistString("telegram.token", token)
}

func (t TelegramConfig) AdminUserID() int64 {
	raw := t.cfg.string("telegram.admin_user_id", "")
	if raw == "" {
		return 0
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func (t TelegramConfig) SetAdminUserID(userID int64) error {
	return t.cfg.persistString("telegram.admin_user_id", strconv.FormatInt(userID, 10))
}

func (t TelegramConfig) PollTimeout() time.Duration {
	return t.cfg.duration("telegram.defaults.poll_timeout_sec", 60, time.Second)
}

func (t TelegramConfig) SetPollTimeout(raw string) error {
	return t.cfg.persistString("telegram.defaults.poll_timeout_sec", strings.TrimSpace(raw))
}

func (t TelegramConfig) DebounceWindow() time.Duration {
	d := t.cfg.duration("telegram.defaults.debounce_ms", 800, time.Millisecond)
	if d <= 0 {
		return 0
	}
	return d
}

func (t TelegramConfig) SetDebounceWindow(raw string) error {
	return t.cfg.persistString("telegram.defaults.debounce_ms", strings.TrimSpace(raw))
}

func (t TelegramConfig) RenderFormat() string {
	format := strings.ToLower(t.cfg.string("telegram.defaults.render_format", "plain"))
	switch format {
	case "", "text", "plain":
		return "plain"
	case "html":
		return "html"
	case "markdown", "markdownv2", "markdown_v2":
		return "markdown_v2"
	default:
		return "plain"
	}
}

func (t TelegramConfig) SetRenderFormat(format string) error {
	return t.cfg.persistString("telegram.defaults.render_format", strings.TrimSpace(format))
}
