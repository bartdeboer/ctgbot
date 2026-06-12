package scheduler

import (
	"context"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

func TestSchedulerRunDueDispatchesScheduledCommand(t *testing.T) {
	ctx := context.Background()
	component := newTestComponent(t)
	if _, err := component.handleJobAdd(ctx, commandengine.Request{}, jobAddCommand{Name: "index", Every: "1h", Command: []string{"do", "work"}}); err != nil {
		t.Fatal(err)
	}

	recorder := &recordingSurface{}
	engine, err := commandset.NewEngineForSource(commandengine.SourceScheduler, recorder)
	if err != nil {
		t.Fatal(err)
	}
	result, err := component.RunDue(ctx, fakeProvider{engine: engine})
	if err != nil {
		t.Fatal(err)
	}
	if result.Due != 1 || result.Succeeded != 1 || result.Failed != 0 {
		t.Fatalf("result = %#v, want one success", result)
	}
	if got, want := strings.Join(recorder.calls, ","), "work"; got != want {
		t.Fatalf("calls = %q, want %q", got, want)
	}

	jobs, err := component.jobs.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].LastStatus != coremodel.ScheduledJobStatusSuccess || jobs[0].LastRunAt == nil || jobs[0].NextRunAt == nil {
		t.Fatalf("jobs = %#v, want completed job", jobs)
	}
}

func TestSchedulerCommandBuilders(t *testing.T) {
	cmdAny, err := buildJobAddCommand(&clir.Request{Params: map[string]string{"name": "nightly"}, Extra: []string{"--every", "24h", "indexing", "run", "default", "--all"}})
	if err != nil {
		t.Fatal(err)
	}
	cmd := cmdAny.(jobAddCommand)
	if cmd.Name != "nightly" || cmd.Every != "24h" || strings.Join(cmd.Command, " ") != "indexing run default --all" {
		t.Fatalf("cmd = %#v", cmd)
	}
}

func newTestComponent(t *testing.T) *Component {
	t.Helper()
	created, err := New(context.Background(), coremodel.Component{Type: Type, Name: Type}, nil, runtimepkg.Profile{Path: t.TempDir()}, repository.NewMemory(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return created.(*Component)
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
