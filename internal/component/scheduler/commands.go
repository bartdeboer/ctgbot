package scheduler

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	schedulerpkg "github.com/bartdeboer/ctgbot/internal/scheduler"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type jobAddCommand struct {
	Name     string
	Every    string
	Cron     string
	Timezone string
	Command  []string
}

type jobListCommand struct{}
type jobRemoveCommand struct{ Name string }

func RegisterGobTypes(register func(any)) {
	register(jobAddCommand{})
	register(jobListCommand{})
	register(jobRemoveCommand{})
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		{
			Pattern: "job add <name>",
			Help:    "Add or replace a scheduled command job",
			Build:   buildJobAddCommand,
			Sources: []commandengine.Source{commandengine.SourceHostbridge, commandengine.SourceMessage},
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
		},
		{
			Pattern: "job list",
			Help:    "List scheduled command jobs",
			Build: func(req *clir.Request) (any, error) {
				return jobListCommand{}, nil
			},
			Sources: []commandengine.Source{commandengine.SourceHostbridge, commandengine.SourceMessage},
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
		},
		{
			Pattern: "job remove <name>",
			Help:    "Remove a scheduled command job",
			Build: func(req *clir.Request) (any, error) {
				name := strings.TrimSpace(req.Params["name"])
				if name == "" {
					return nil, fmt.Errorf("missing job name")
				}
				return jobRemoveCommand{Name: name}, nil
			},
			Sources: []commandengine.Source{commandengine.SourceHostbridge, commandengine.SourceMessage},
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
		},
	}
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := commandengine.Register[jobAddCommand](registry, c.handleJobAdd); err != nil {
		return err
	}
	if err := commandengine.Register[jobListCommand](registry, c.handleJobList); err != nil {
		return err
	}
	if err := commandengine.Register[jobRemoveCommand](registry, c.handleJobRemove); err != nil {
		return err
	}
	return nil
}

func buildJobAddCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("scheduler job add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	every := fs.String("every", "", "Run interval, for example 24h")
	cronExpr := fs.String("cron", "", "Cron schedule, for example '30 1 * * *'")
	timezone := fs.String("tz", "", "Cron timezone, for example Europe/Amsterdam")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	argv := append([]string{}, fs.Args()...)
	if len(argv) > 0 && argv[0] == "--" {
		argv = argv[1:]
	}
	cmd := jobAddCommand{
		Name:     strings.TrimSpace(req.Params["name"]),
		Every:    strings.TrimSpace(*every),
		Cron:     strings.TrimSpace(*cronExpr),
		Timezone: strings.TrimSpace(*timezone),
		Command:  argv,
	}
	if cmd.Name == "" {
		return nil, fmt.Errorf("missing job name")
	}
	if cmd.Every == "" && cmd.Cron == "" {
		return nil, fmt.Errorf("missing --every or --cron")
	}
	if cmd.Every != "" && cmd.Cron != "" {
		return nil, fmt.Errorf("use either --every or --cron, not both")
	}
	if cmd.Timezone != "" && cmd.Cron == "" {
		return nil, fmt.Errorf("--tz requires --cron")
	}
	if len(cmd.Command) == 0 {
		return nil, fmt.Errorf("missing scheduled command")
	}
	return cmd, nil
}

func (c *Component) handleJobAdd(ctx context.Context, req commandengine.Request, cmd jobAddCommand) (commandengine.Result, error) {
	job, err := schedulerpkg.NewScheduledJob(cmd.Name, schedulerpkg.JobSchedule{Every: cmd.Every, Cron: cmd.Cron, Timezone: cmd.Timezone}, cmd.Command, time.Now().UTC())
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := c.jobs.Save(ctx, &job); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("scheduled job saved: %s", job.Name)}, nil
}

func (c *Component) handleJobList(ctx context.Context, req commandengine.Request, cmd jobListCommand) (commandengine.Result, error) {
	jobs, err := c.jobs.List(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	if len(jobs) == 0 {
		return commandengine.Result{Text: "no scheduled jobs"}, nil
	}
	var lines []string
	for _, job := range jobs {
		var argv []string
		_ = json.Unmarshal([]byte(job.CommandJSON), &argv)
		line := fmt.Sprintf("%s %s enabled=%t next=%s status=%s command=%s", job.Name, formatSchedule(job), job.Enabled, formatTime(job.NextRunAt), job.LastStatus, strings.Join(argv, " "))
		if strings.TrimSpace(job.LastError) != "" {
			line += " error=" + job.LastError
		}
		lines = append(lines, line)
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func formatSchedule(job coremodel.ScheduledJob) string {
	if every := strings.TrimSpace(job.Every); every != "" {
		return "every=" + every
	}
	parts := []string{"cron=" + fmt.Sprintf("%q", strings.TrimSpace(job.Cron))}
	if timezone := strings.TrimSpace(job.Timezone); timezone != "" {
		parts = append(parts, "tz="+timezone)
	}
	return strings.Join(parts, " ")
}

func (c *Component) handleJobRemove(ctx context.Context, req commandengine.Request, cmd jobRemoveCommand) (commandengine.Result, error) {
	deleted, err := c.jobs.DeleteByName(ctx, cmd.Name)
	if err != nil {
		return commandengine.Result{}, err
	}
	if !deleted {
		return commandengine.Result{Text: "scheduled job not found: " + cmd.Name}, nil
	}
	return commandengine.Result{Text: "scheduled job removed: " + cmd.Name}, nil
}

func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
