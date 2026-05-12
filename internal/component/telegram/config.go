package telegram

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	TokenFilename           = "token.txt"
	ComponentConfigFilename = "component.json"

	defaultPollTimeout    = time.Minute
	defaultDebounceWindow = 800 * time.Millisecond
	defaultRenderFormat   = "plain"
)

type ComponentConfig struct {
	Operators      []int64 `json:"operators,omitempty"`
	PollTimeout    string  `json:"poll_timeout,omitempty"`
	DebounceWindow string  `json:"debounce_window,omitempty"`
	RenderFormat   string  `json:"render_format,omitempty"`
}

func loadToken(homePath string) (string, error) {
	path := filepath.Join(strings.TrimSpace(homePath), TokenFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read telegram token %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func loadComponentConfig(homePath string) (ComponentConfig, error) {
	path := filepath.Join(strings.TrimSpace(homePath), ComponentConfigFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ComponentConfig{}.withDefaults(), nil
		}
		return ComponentConfig{}, fmt.Errorf("read telegram component config %s: %w", path, err)
	}
	var config ComponentConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return ComponentConfig{}, fmt.Errorf("read telegram component config %s: %w", path, err)
	}
	return config.withDefaults(), nil
}

func (c ComponentConfig) withDefaults() ComponentConfig {
	c.Operators = sanitizeTelegramUserIDs(c.Operators)
	c.PollTimeout = strings.TrimSpace(c.PollTimeout)
	if c.PollTimeout == "" {
		c.PollTimeout = defaultPollTimeout.String()
	}
	c.DebounceWindow = strings.TrimSpace(c.DebounceWindow)
	if c.DebounceWindow == "" {
		c.DebounceWindow = defaultDebounceWindow.String()
	}
	c.RenderFormat = normalizeTelegramRenderFormat(c.RenderFormat)
	return c
}

func (c ComponentConfig) pollTimeout() time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(c.PollTimeout))
	if err != nil || d <= 0 {
		return defaultPollTimeout
	}
	return d
}

func (c ComponentConfig) debounceWindow() time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(c.DebounceWindow))
	if err != nil {
		return defaultDebounceWindow
	}
	if d <= 0 {
		return 0
	}
	return d
}

func (c ComponentConfig) renderFormat() string {
	return normalizeTelegramRenderFormat(c.RenderFormat)
}

func normalizeTelegramRenderFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
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
