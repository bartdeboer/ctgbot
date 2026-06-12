package scheduler

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	schedulerpkg "github.com/bartdeboer/ctgbot/internal/scheduler"
)

const Type = "scheduler"

type ScheduledCommandEngineProvider = schedulerpkg.ScheduledCommandEngineProvider

type Component struct {
	registration coremodel.Component
	jobs         repository.ScheduledJobRepository
	logf         func(format string, args ...any)
}

var _ component.Component = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)
var _ component.LocalCommandSurface = (*Component)(nil)

func New(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, profile runtimepkg.Profile, storage repository.Storage, logf func(format string, args ...any)) (component.Component, error) {
	_, _, _ = ctx, runtime, profile
	if storage == nil {
		return nil, fmt.Errorf("missing scheduler storage")
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Component{registration: registration, jobs: storage.ScheduledJobs(), logf: logf}, nil
}

func (c *Component) Type() string { return Type }

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) RunDue(ctx context.Context, provider ScheduledCommandEngineProvider) (schedulerpkg.RunDueResult, error) {
	if c == nil || c.jobs == nil {
		return schedulerpkg.RunDueResult{}, fmt.Errorf("missing scheduled job repository")
	}
	if provider == nil {
		return schedulerpkg.RunDueResult{}, fmt.Errorf("missing scheduled command engine provider")
	}
	return schedulerpkg.RunDue(ctx, c.jobs, provider, c.logf)
}

type RunDueResult = schedulerpkg.RunDueResult
