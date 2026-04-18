package durationparse

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func Parse(raw string, defaultUnit time.Duration) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return time.Duration(n) * defaultUnit, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", raw, err)
	}
	return d, nil
}
