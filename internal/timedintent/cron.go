package timedintent

import (
	"time"

	"github.com/robfig/cron/v3"

	"github.com/bartdeboer/ctgbot/internal/schedule"
)

const DefaultTimezone = schedule.DefaultTimezone

func nextCronDue(expr string, timezone string, after time.Time) (time.Time, error) {
	return schedule.NextCron(expr, timezone, after)
}

func parseCron(expr string, timezone string) (cron.Schedule, error) {
	return schedule.ParseCron(expr, timezone)
}

func hasCronTimezone(expr string) bool {
	return schedule.HasCronTimezone(expr)
}
