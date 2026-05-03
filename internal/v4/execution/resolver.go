package execution

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	v4component "github.com/bartdeboer/ctgbot/internal/v4/component"
	"github.com/bartdeboer/ctgbot/internal/v4/profiles"
	"github.com/bartdeboer/ctgbot/internal/v4/workspaces"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

type Factory interface {
	Driver() string
	Create(ctx context.Context, profile profiles.Profile, req CreateRequest) (v4component.Runtime, error)
}

type CreateRequest struct {
	Config     *appstate.Config
	Sandboxes  sandboxengine.RuntimeManager
	Workspaces *workspaces.Manager
	Logger     *log.Logger
}

type Resolver struct {
	factories map[string]Factory
	req       CreateRequest

	mu       sync.Mutex
	runtimes map[string]v4component.Runtime
}

func NewResolver(req CreateRequest, factories ...Factory) *Resolver {
	r := &Resolver{
		factories: map[string]Factory{},
		req:       req,
	}
	for _, factory := range factories {
		_ = r.Add(factory)
	}
	return r
}

func (r *Resolver) Add(factory Factory) error {
	if r == nil {
		return fmt.Errorf("missing runtime resolver")
	}
	if factory == nil {
		return fmt.Errorf("missing runtime factory")
	}
	driver := strings.TrimSpace(factory.Driver())
	if driver == "" {
		return fmt.Errorf("missing runtime driver")
	}
	if r.factories == nil {
		r.factories = map[string]Factory{}
	}
	if _, exists := r.factories[driver]; exists {
		return fmt.Errorf("runtime factory already registered: %s", driver)
	}
	r.factories[driver] = factory
	return nil
}

func (r *Resolver) Resolve(ctx context.Context, profile profiles.Profile) (v4component.Runtime, error) {
	if r == nil {
		return nil, fmt.Errorf("missing runtime resolver")
	}
	profileName := strings.TrimSpace(profile.Name)
	if profileName == "" {
		return nil, fmt.Errorf("missing runtime profile")
	}
	r.mu.Lock()
	if runtime := r.runtimes[profileName]; runtime != nil {
		r.mu.Unlock()
		return runtime, nil
	}
	r.mu.Unlock()

	driver := strings.TrimSpace(profile.Runtime)
	if driver == "" {
		return nil, fmt.Errorf("missing runtime driver for profile %s", profile.Name)
	}
	factory, ok := r.factories[driver]
	if !ok {
		return nil, fmt.Errorf("runtime driver not registered: %s", driver)
	}
	runtime, err := factory.Create(ctx, profile, r.req)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	if r.runtimes == nil {
		r.runtimes = map[string]v4component.Runtime{}
	}
	r.runtimes[profileName] = runtime
	r.mu.Unlock()
	return runtime, nil
}
