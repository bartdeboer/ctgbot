package scheduler

import (
	"fmt"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/robfig/cron/v3"
)

const (
	ScheduleTypeInterval = "interval"
	ScheduleTypeCron     = "cron"

	DefaultTimezone = "UTC"
	MinimumInterval = time.Minute
)

func NewCronJob(name string, expr string, timezone string, argv []string, now time.Time) (coremodel.ScheduledJob, error) {
	commandJSON, err := commandJSON(argv)
	if err != nil {
		return coremodel.ScheduledJob{}, err
	}
	job := coremodel.ScheduledJob{
		Name:         name,
		Enabled:      true,
		ScheduleType: ScheduleTypeCron,
		Cron:         expr,
		Timezone:     timezone,
		CommandJSON:  commandJSON,
	}
	if err := PrepareJob(&job, now); err != nil {
		return coremodel.ScheduledJob{}, err
	}
	return job, nil
}

func normalizeSchedule(job *coremodel.ScheduledJob) {
	job.ScheduleType = strings.TrimSpace(job.ScheduleType)
	job.Every = strings.TrimSpace(job.Every)
	job.Cron = strings.TrimSpace(job.Cron)
	job.Timezone = strings.TrimSpace(job.Timezone)
	if job.ScheduleType == "" {
		if job.Cron != "" {
			job.ScheduleType = ScheduleTypeCron
		} else {
			job.ScheduleType = ScheduleTypeInterval
		}
	}
	if job.ScheduleType == ScheduleTypeCron && job.Timezone == "" && !hasCronTimezone(job.Cron) {
		job.Timezone = DefaultTimezone
	}
}

func validateSchedule(job coremodel.ScheduledJob) error {
	switch job.ScheduleType {
	case ScheduleTypeInterval:
		if job.Every == "" {
			return fmt.Errorf("missing schedule interval")
		}
		every, err := time.ParseDuration(job.Every)
		if err != nil {
			return fmt.Errorf("parse --every: %w", err)
		}
		if every < MinimumInterval {
			return fmt.Errorf("interval must be at least %s", MinimumInterval)
		}
		return nil
	case ScheduleTypeCron:
		if job.Cron == "" {
			return fmt.Errorf("missing cron expression")
		}
		_, err := cronSchedule(job)
		return err
	default:
		return fmt.Errorf("unsupported schedule type: %s", job.ScheduleType)
	}
}

func nextRun(job coremodel.ScheduledJob, after time.Time) (time.Time, error) {
	if after.IsZero() {
		after = time.Now().UTC()
	}
	switch job.ScheduleType {
	case ScheduleTypeInterval:
		every, err := time.ParseDuration(job.Every)
		if err != nil {
			return time.Time{}, err
		}
		return after.UTC().Add(every), nil
	case ScheduleTypeCron:
		schedule, err := cronSchedule(job)
		if err != nil {
			return time.Time{}, err
		}
		next := schedule.Next(after.UTC())
		if next.IsZero() {
			return time.Time{}, fmt.Errorf("cron expression has no next run")
		}
		return next.UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported schedule type: %s", job.ScheduleType)
	}
}

func cronSchedule(job coremodel.ScheduledJob) (cron.Schedule, error) {
	spec := strings.TrimSpace(job.Cron)
	if spec == "" {
		return nil, fmt.Errorf("missing cron expression")
	}
	timezone := strings.TrimSpace(job.Timezone)
	if hasCronTimezone(spec) {
		if timezone != "" {
			return nil, fmt.Errorf("use either --tz or CRON_TZ in cron expression, not both")
		}
		return cron.ParseStandard(spec)
	}
	if timezone == "" {
		timezone = DefaultTimezone
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", timezone, err)
	}
	return cron.ParseStandard("CRON_TZ=" + timezone + " " + spec)
}

func hasCronTimezone(spec string) bool {
	spec = strings.TrimSpace(spec)
	return strings.HasPrefix(spec, "TZ=") || strings.HasPrefix(spec, "CRON_TZ=")
}
