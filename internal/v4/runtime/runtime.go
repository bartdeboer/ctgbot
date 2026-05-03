package runtime

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/dbstorage"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	v4broker "github.com/bartdeboer/ctgbot/internal/v4/broker"
	v4component "github.com/bartdeboer/ctgbot/internal/v4/component"
	"github.com/bartdeboer/ctgbot/internal/v4/execution"
	"github.com/bartdeboer/ctgbot/internal/v4/homes"
	"github.com/bartdeboer/ctgbot/internal/v4/profiles"
	"github.com/bartdeboer/ctgbot/internal/v4/repository"
	"github.com/bartdeboer/ctgbot/internal/v4/workspaces"
	"gorm.io/gorm"
)

type Runtime struct {
	Storage    repository.Storage
	Registry   *v4component.Registry
	Profiles   *profiles.Manager
	Homes      *homes.Manager
	Runtimes   *execution.Resolver
	Workspaces *workspaces.Manager

	StateRoot       string
	DBPath          string
	Config          *appstate.Config
	TelegramUpdates dbstorage.TelegramUpdateStorage
	Sandboxes       sandboxengine.RuntimeManager
	Logger          *log.Logger
	DB              *gorm.DB

	mu        sync.Mutex
	instances map[modeluuid.UUID]*v4component.Instance
}

func New(storage repository.Storage, registry *v4component.Registry, profileManager *profiles.Manager, homeManager *homes.Manager, runtimes *execution.Resolver) *Runtime {
	return &Runtime{
		Storage:  storage,
		Registry: registry,
		Profiles: profileManager,
		Homes:    homeManager,
		Runtimes: runtimes,
	}
}

func (r *Runtime) ResolveComponent(ctx context.Context, componentID modeluuid.UUID) (*v4component.Instance, error) {
	if r == nil || r.Storage == nil {
		return nil, fmt.Errorf("missing runtime storage")
	}
	if r.Registry == nil {
		return nil, fmt.Errorf("missing component registry")
	}
	if r.Profiles == nil {
		return nil, fmt.Errorf("missing profile manager")
	}
	if r.Homes == nil {
		return nil, fmt.Errorf("missing component homes")
	}
	if r.Runtimes == nil {
		return nil, fmt.Errorf("missing runtime resolver")
	}
	r.mu.Lock()
	if instance := r.instances[componentID]; instance != nil {
		r.mu.Unlock()
		return instance, nil
	}
	r.mu.Unlock()
	componentRow, err := r.Storage.Components().GetByID(ctx, componentID)
	if err != nil {
		return nil, err
	}
	if componentRow == nil {
		return nil, fmt.Errorf("component not found: %s", componentID)
	}
	profile, err := r.Profiles.Resolve(componentRow.Profile)
	if err != nil {
		return nil, err
	}
	home, err := r.Homes.Ensure(*componentRow)
	if err != nil {
		return nil, err
	}
	runtime, err := r.Runtimes.Resolve(ctx, profile)
	if err != nil {
		return nil, err
	}
	instance, err := r.Registry.InstantiateWithRuntime(ctx, *componentRow, profile, home, runtime, r.Storage)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	if r.instances == nil {
		r.instances = map[modeluuid.UUID]*v4component.Instance{}
	}
	r.instances[componentID] = instance
	r.mu.Unlock()
	return instance, nil
}

func (r *Runtime) Broker(logf func(format string, args ...any)) *v4broker.Broker {
	return v4broker.New(r.Storage, r, logf)
}
