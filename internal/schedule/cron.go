package schedule

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

const DefaultTimezone = "UTC"

func NextCron(expr string, timezone string, after time.Time) (time.Time, error) {
	schedule, err := ParseCron(expr, timezone)
	if err != nil {
		return time.Time{}, err
	}
	if after.IsZero() {
		after = time.Now().UTC()
	}
	next := schedule.Next(after.UTC())
	if next.IsZero() {
		return time.Time{}, fmt.Errorf("cron expression has no next run")
	}
	return next.UTC(), nil
}

func ParseCron(expr string, timezone string) (cron.Schedule, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("missing cron expression")
	}
	timezone = strings.TrimSpace(timezone)
	if HasCronTimezone(expr) {
		if timezone != "" {
			return nil, fmt.Errorf("use either --tz or CRON_TZ in cron expression, not both")
		}
		return cron.ParseStandard(expr)
	}
	if timezone == "" {
		timezone = DefaultTimezone
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", timezone, err)
	}
	return cron.ParseStandard("CRON_TZ=" + timezone + " " + expr)
}

func HasCronTimezone(expr string) bool {
	expr = strings.TrimSpace(expr)
	return strings.HasPrefix(expr, "TZ=") || strings.HasPrefix(expr, "CRON_TZ=")
}
