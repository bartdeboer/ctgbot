package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/repository"
	"github.com/bartdeboer/ctgbot/internal/schedule"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/ctgbot/internal/timedintent"
)

const DefaultInterval = time.Minute

type App interface {
	ScheduledCommandEngine(ctx context.Context) (*commandengine.Engine, error)
	ScheduledJobRepository() repository.ScheduledJobRepository
}

type Scheduler struct {
	app      App
	interval time.Duration
	logf     func(format string, args ...any)
	timed    *timedintent.Service
}

type Option func(*Scheduler)

func WithInterval(interval time.Duration) Option {
	return func(s *Scheduler) {
		if interval > 0 {
			s.interval = interval
		}
	}
}

func WithTimedIntentService(service *timedintent.Service) Option {
	return func(s *Scheduler) {
		s.timed = service
	}
}

func New(app App, logf func(format string, args ...any), opts ...Option) *Scheduler {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	s := &Scheduler{app: app, interval: DefaultInterval, logf: logf}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

func (s *Scheduler) Run(ctx context.Context) error {
	if s == nil || s.app == nil {
		return fmt.Errorf("missing scheduler app")
	}
	if _, err := s.RunDue(ctx); err != nil {
		s.logf("scheduler run due failed: %v", err)
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := s.RunDue(ctx); err != nil {
				s.logf("scheduler run due failed: %v", err)
			}
		}
	}
}

func (s *Scheduler) RunDue(ctx context.Context) (RunDueResult, error) {
	if s == nil || s.app == nil {
		return RunDueResult{}, fmt.Errorf("missing scheduler app")
	}
	result, err := RunDue(ctx, s.app.ScheduledJobRepository(), s.app, s.logf)
	if s.timed != nil {
		timedResult, timedErr := s.timed.RunDue(ctx)
		result.TimedDue = timedResult.Due
		result.TimedDelivered = timedResult.Delivered
		result.TimedSkippedBusy = timedResult.SkippedBusy
		result.TimedFailed = timedResult.Failed
		result.TimedExpired = timedResult.Expired
		if err == nil {
			err = timedErr
		} else if timedErr != nil {
			s.logf("timed intents run due failed: %v", timedErr)
		}
	}
	return result, err
}

type ScheduledCommandEngineProvider interface {
	ScheduledCommandEngine(ctx context.Context) (*commandengine.Engine, error)
}

type RunDueResult struct {
	Due              int
	Succeeded        int
	Failed           int
	TimedDue         int
	TimedDelivered   int
	TimedSkippedBusy int
	TimedFailed      int
	TimedExpired     int
}

func RunDue(ctx context.Context, jobs repository.ScheduledJobRepository, provider ScheduledCommandEngineProvider, logf func(format string, args ...any)) (RunDueResult, error) {
	if jobs == nil {
		return RunDueResult{}, fmt.Errorf("missing scheduled job repository")
	}
	if provider == nil {
		return RunDueResult{}, fmt.Errorf("missing scheduled command engine provider")
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	due, err := jobs.ListDue(ctx, time.Now().UTC())
	if err != nil {
		return RunDueResult{}, err
	}
	result := RunDueResult{Due: len(due)}
	if len(due) == 0 {
		return result, nil
	}
	engine, err := provider.ScheduledCommandEngine(ctx)
	if err != nil {
		return result, err
	}
	for _, job := range due {
		runErr := RunJob(ctx, engine, job)
		if err := FinishJob(ctx, jobs, job, runErr, time.Now().UTC()); err != nil {
			return result, err
		}
		if runErr != nil {
			result.Failed++
			logf("scheduler job failed name=%s err=%v", job.Name, runErr)
		} else {
			result.Succeeded++
		}
	}
	return result, nil
}

func RunJob(ctx context.Context, engine *commandengine.Engine, job coremodel.ScheduledJob) error {
	if engine == nil {
		return fmt.Errorf("missing scheduled command engine")
	}
	argv, err := Argv(job)
	if err != nil {
		return err
	}
	base := commandengine.Request{Context: commandengine.Context{
		Source: commandengine.SourceScheduler,
		Actor:  coremodel.Actor{ID: "scheduler", Label: "scheduler", Roles: []simplerbac.Role{simplerbac.RoleRoot}},
	}}
	_, err = engine.Run(ctx, base, argv)
	return err
}

type JobSchedule struct {
	Every    string
	Cron     string
	Timezone string
}

func NewJob(name string, every string, argv []string, now time.Time) (coremodel.ScheduledJob, error) {
	return NewScheduledJob(name, JobSchedule{Every: every}, argv, now)
}

func NewScheduledJob(name string, spec JobSchedule, argv []string, now time.Time) (coremodel.ScheduledJob, error) {
	commandJSON, err := json.Marshal(argv)
	if err != nil {
		return coremodel.ScheduledJob{}, err
	}
	job := coremodel.ScheduledJob{Name: name, Enabled: true, Every: spec.Every, Cron: spec.Cron, Timezone: spec.Timezone, CommandJSON: string(commandJSON)}
	if err := PrepareJob(&job, now); err != nil {
		return coremodel.ScheduledJob{}, err
	}
	return job, nil
}

func PrepareJob(job *coremodel.ScheduledJob, now time.Time) error {
	if job == nil {
		return fmt.Errorf("missing scheduled job")
	}
	job.Name = strings.TrimSpace(job.Name)
	job.Every = strings.TrimSpace(job.Every)
	job.Cron = strings.TrimSpace(job.Cron)
	job.Timezone = strings.TrimSpace(job.Timezone)
	job.CommandJSON = strings.TrimSpace(job.CommandJSON)
	if job.Name == "" {
		return fmt.Errorf("missing job name")
	}
	if job.Every == "" && job.Cron == "" {
		return fmt.Errorf("missing schedule interval")
	}
	if job.Every != "" && job.Cron != "" {
		return fmt.Errorf("use either --every or --cron, not both")
	}
	if job.Every != "" {
		if job.Timezone != "" {
			return fmt.Errorf("--tz requires --cron")
		}
		if _, err := time.ParseDuration(job.Every); err != nil {
			return fmt.Errorf("parse --every: %w", err)
		}
	}
	if job.Cron != "" {
		if _, err := schedule.ParseCron(job.Cron, job.Timezone); err != nil {
			return fmt.Errorf("parse --cron: %w", err)
		}
	}
	if _, err := Argv(*job); err != nil {
		return err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if job.NextRunAt == nil {
		next, err := firstRunAt(*job, now)
		if err != nil {
			return err
		}
		job.NextRunAt = &next
	}
	if job.LastStatus == "" {
		job.LastStatus = coremodel.ScheduledJobStatusNever
	}
	return nil
}

func firstRunAt(job coremodel.ScheduledJob, now time.Time) (time.Time, error) {
	if strings.TrimSpace(job.Cron) != "" {
		return schedule.NextCron(job.Cron, job.Timezone, now)
	}
	return now.UTC(), nil
}

func nextIntervalRunAt(job coremodel.ScheduledJob, every time.Duration, finishedAt time.Time) time.Time {
	base := finishedAt.UTC()
	if job.NextRunAt != nil && !job.NextRunAt.IsZero() {
		base = job.NextRunAt.UTC()
	}
	next := base.Add(every)
	for !next.After(finishedAt.UTC()) {
		next = next.Add(every)
	}
	return next
}

func Argv(job coremodel.ScheduledJob) ([]string, error) {
	var argv []string
	if err := json.Unmarshal([]byte(job.CommandJSON), &argv); err != nil {
		return nil, err
	}
	if len(argv) == 0 {
		return nil, fmt.Errorf("scheduled job %s has no command", job.Name)
	}
	return argv, nil
}

func FinishJob(ctx context.Context, jobs repository.ScheduledJobRepository, job coremodel.ScheduledJob, runErr error, finishedAt time.Time) error {
	if jobs == nil {
		return fmt.Errorf("missing scheduled job repository")
	}
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}
	next, err := nextRunAt(job, finishedAt)
	if err != nil {
		return err
	}
	job.LastRunAt = &finishedAt
	job.NextRunAt = &next
	if runErr != nil {
		job.LastStatus = coremodel.ScheduledJobStatusFailed
		job.LastError = runErr.Error()
	} else {
		job.LastStatus = coremodel.ScheduledJobStatusSuccess
		job.LastError = ""
	}
	return jobs.Save(ctx, &job)
}

func nextRunAt(job coremodel.ScheduledJob, finishedAt time.Time) (time.Time, error) {
	if strings.TrimSpace(job.Cron) != "" {
		return schedule.NextCron(job.Cron, job.Timezone, finishedAt)
	}
	every, err := time.ParseDuration(job.Every)
	if err != nil {
		return time.Time{}, err
	}
	return nextIntervalRunAt(job, every, finishedAt), nil
}
