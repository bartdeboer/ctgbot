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
	v3broker "github.com/bartdeboer/ctgbot/internal/v3/broker"
	v3component "github.com/bartdeboer/ctgbot/internal/v3/component"
	"github.com/bartdeboer/ctgbot/internal/v3/homes"
	"github.com/bartdeboer/ctgbot/internal/v3/repository"
	"github.com/bartdeboer/ctgbot/internal/v3/workspaces"
	"gorm.io/gorm"
)

type Runtime struct {
	Storage    repository.Storage
	Registry   *v3component.Registry
	Homes      *homes.Manager
	Workspaces *workspaces.Manager

	StateRoot       string
	DBPath          string
	Config          *appstate.Config
	TelegramUpdates dbstorage.TelegramUpdateStorage
	Sandboxes       sandboxengine.RuntimeManager
	Logger          *log.Logger
	DB              *gorm.DB

	mu        sync.Mutex
	instances map[modeluuid.UUID]*v3component.Instance
}

func New(storage repository.Storage, registry *v3component.Registry, homeManager *homes.Manager) *Runtime {
	return &Runtime{
		Storage:  storage,
		Registry: registry,
		Homes:    homeManager,
	}
}

func (r *Runtime) ResolveComponent(ctx context.Context, componentID modeluuid.UUID) (*v3component.Instance, error) {
	if r == nil || r.Storage == nil {
		return nil, fmt.Errorf("missing runtime storage")
	}
	if r.Registry == nil {
		return nil, fmt.Errorf("missing component registry")
	}
	if r.Homes == nil {
		return nil, fmt.Errorf("missing component homes")
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
	home, err := r.Homes.Ensure(*componentRow)
	if err != nil {
		return nil, err
	}
	instance, err := r.Registry.Instantiate(ctx, *componentRow, home, r.Storage)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	if r.instances == nil {
		r.instances = map[modeluuid.UUID]*v3component.Instance{}
	}
	r.instances[componentID] = instance
	r.mu.Unlock()
	return instance, nil
}

func (r *Runtime) Broker(logf func(format string, args ...any)) *v3broker.Broker {
	return v3broker.New(r.Storage, r, logf)
}
