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

type telegramConfigValue struct {
	Token     string  `json:"token"`
	Operators []int64 `json:"operators"`
}

type legacyOperatorConfigValue struct {
	TelegramUserIDs []int64 `json:"telegram_user_ids"`
}

func (t TelegramConfig) Token() string {
	if token := t.cfg.string("telegram.token", ""); token != "" {
		return token
	}
	var value telegramConfigValue
	if t.cfg.structValue("telegram", &value) {
		return strings.TrimSpace(value.Token)
	}
	return ""
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

func (t TelegramConfig) OperatorUserIDs() []int64 {
	var telegramValue telegramConfigValue
	if t.cfg.structValue("telegram", &telegramValue) {
		ids := sanitizeTelegramUserIDs(telegramValue.Operators)
		if len(ids) > 0 {
			return ids
		}
	}

	var legacyOperators legacyOperatorConfigValue
	if t.cfg.structValue("operators", &legacyOperators) {
		ids := sanitizeTelegramUserIDs(legacyOperators.TelegramUserIDs)
		if len(ids) > 0 {
			return ids
		}
	}

	if adminID := t.AdminUserID(); adminID != 0 {
		return []int64{adminID}
	}
	return nil
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

func sanitizeTelegramUserIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	out := make([]int64, 0, len(ids))
	seen := map[int64]struct{}{}
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
