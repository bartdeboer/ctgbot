package scheduler

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/repository"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

func TestRunDueQueuesJobsSequentially(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)

	for _, name := range []string{"b", "a"} {
		job, err := NewJob(name, "1h", []string{"do", name}, now)
		if err != nil {
			t.Fatal(err)
		}
		if err := storage.ScheduledJobs().Save(ctx, &job); err != nil {
			t.Fatal(err)
		}
	}

	surface := &recordingSurface{}
	engine, err := commandset.NewEngineForSource(commandengine.SourceScheduler, surface)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunDue(ctx, storage.ScheduledJobs(), fakeProvider{engine: engine}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Due != 2 || result.Succeeded != 2 || result.Failed != 0 {
		t.Fatalf("result = %#v, want two successes", result)
	}
	if got, want := strings.Join(surface.calls, ","), "a,b"; got != want {
		t.Fatalf("calls = %q, want %q", got, want)
	}

	jobs, err := storage.ScheduledJobs().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range jobs {
		if job.LastStatus != coremodel.ScheduledJobStatusSuccess || job.LastRunAt == nil || job.NextRunAt == nil {
			t.Fatalf("job = %#v, want completed job", job)
		}
	}
}

type fakeProvider struct{ engine *commandengine.Engine }

func (p fakeProvider) ScheduledCommandEngine(ctx context.Context) (*commandengine.Engine, error) {
	_ = ctx
	return p.engine, nil
}

type recordingSurface struct{ calls []string }

var _ component.CommandSurface = (*recordingSurface)(nil)

func (s *recordingSurface) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{{
		Pattern: "do <name>",
		Help:    "record a scheduled command",
		Sources: []commandengine.Source{commandengine.SourceScheduler},
		Policy:  simplerbac.Any(simplerbac.RoleRoot),
		Build: func(req *clir.Request) (any, error) {
			return recordCommand{Name: req.Params["name"]}, nil
		},
	}}
}

func (s *recordingSurface) RegisterCommandHandlers(registry *commandengine.Registry) error {
	return commandengine.Register[recordCommand](registry, func(ctx context.Context, req commandengine.Request, cmd recordCommand) (commandengine.Result, error) {
		s.calls = append(s.calls, cmd.Name)
		return commandengine.Result{Text: "ok"}, nil
	})
}

type recordCommand struct{ Name string }
