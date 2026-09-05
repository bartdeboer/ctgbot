package backend

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bartdeboer/ctgbot/internal/containerengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

const Kind = "backend"

type Factory struct {
	componentsRoot string
	logger         *log.Logger
	env            []string
	owners         *nativeOwners
}

type ServiceSpec struct {
	Native    *NativeConfig
	Identity  string // Stable component/model identity, independent of display/container names.
	BaseURL   string
	HealthURL string
	Ports     []string
	Env       []string
	Mounts    []containerengine.Mount
	Cmd       []string
}

type Binder interface {
	runtimepkg.Factory
	BindBackend(registration coremodel.Component, profile runtimepkg.Profile, config runtimepkg.BindConfig, service ServiceSpec) (runtimepkg.ServiceRuntime, error)
}

func New(componentsRoot string, logger *log.Logger) *Factory {
	shutdown, cancel := context.WithCancel(context.Background())
	return &Factory{
		componentsRoot: strings.TrimSpace(componentsRoot),
		logger:         logger,
		owners:         &nativeOwners{services: make(map[string]*nativeRuntime), shutdown: shutdown, cancel: cancel},
	}
}

func (f *Factory) WithEnv(env ...string) *Factory {
	if f == nil {
		return nil
	}
	clone := *f
	clone.env = runtimepkg.MergeEnv(clone.env, env)
	return &clone
}

func (f *Factory) Kind() string {
	return Kind
}

func (f *Factory) ComponentProfile(registration coremodel.Component) runtimepkg.Profile {
	hostPath := strings.TrimSpace(registration.ProfilePath)
	if hostPath == "" {
		hostPath = filepath.Join(f.componentsRoot, registration.Type, registration.Name)
	}
	return runtimepkg.Profile{Path: hostPath}
}

func (f *Factory) RuntimeComponentProfilePath(registration coremodel.Component, profile runtimepkg.Profile) string {
	_, _ = registration, profile
	return strings.TrimSpace(profile.Path)
}

func (f *Factory) RuntimeWorkspacePath(workspacePath string) string {
	return strings.TrimSpace(workspacePath)
}

func (f *Factory) BindBackend(
	registration coremodel.Component,
	profile runtimepkg.Profile,
	config runtimepkg.BindConfig,
	service ServiceSpec,
) (runtimepkg.ServiceRuntime, error) {
	config = config.WithEnvOverride(f.env...)
	if service.Native != nil {
		return f.bindNative(profile, config, service)
	}
	return &Runtime{
		registration: registration,
		profile:      profile,
		config:       config,
		service:      service.clean(),
		containers:   containerengine.NewManager(f.logger),
	}, nil
}

type Runtime struct {
	registration coremodel.Component
	profile      runtimepkg.Profile
	config       runtimepkg.BindConfig
	service      ServiceSpec
	containers   *containerengine.Manager
}

func (r *Runtime) ComponentProfile() runtimepkg.Profile {
	if r == nil {
		return runtimepkg.Profile{}
	}
	return r.profile
}

func (r *Runtime) BaseURL() string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.service.BaseURL)
}

func (r *Runtime) Start(ctx context.Context) (runtimepkg.Status, error) {
	container := r.container()
	state, err := container.InspectState(ctx)
	if err != nil {
		return runtimepkg.Status{}, err
	}
	if state == containerengine.StateMissing {
		spec, err := r.containerSpec()
		if err != nil {
			return runtimepkg.Status{}, err
		}
		if _, err := r.containers.Create(ctx, spec); err != nil {
			return runtimepkg.Status{}, err
		}
		state = containerengine.StateCreated
	}
	if state != containerengine.StateRunning {
		if err := container.Start(ctx); err != nil {
			return runtimepkg.Status{}, err
		}
	}
	if err := r.waitReady(ctx); err != nil {
		return runtimepkg.Status{}, err
	}
	return r.Status(ctx)
}

func (r *Runtime) Stop(ctx context.Context) error {
	return r.container().Stop(ctx)
}

func (r *Runtime) Refresh(ctx context.Context) error {
	return r.container().Remove(ctx)
}

func (r *Runtime) Status(ctx context.Context) (runtimepkg.Status, error) {
	state, err := r.container().InspectState(ctx)
	if err != nil {
		return runtimepkg.Status{}, err
	}
	return runtimepkg.Status{
		Name:               r.containerName(),
		State:              string(state),
		RuntimeProfilePath: strings.TrimSpace(r.profile.Path),
	}, nil
}

func (r *Runtime) container() *containerengine.Container {
	if r == nil || r.containers == nil {
		return &containerengine.Container{}
	}
	return r.containers.Container(r.containerName())
}

func (r *Runtime) containerSpec() (containerengine.ContainerSpec, error) {
	securityOpts, err := containerengine.SeccompSecurityOpts(r.config.Seccomp)
	if err != nil {
		return containerengine.ContainerSpec{}, err
	}
	env := runtimepkg.MergeEnv(r.service.Env, r.config.Env)
	return containerengine.ContainerSpec{
		Name:         r.containerName(),
		Image:        strings.TrimSpace(r.config.Image),
		Entrypoint:   strings.TrimSpace(r.config.Entrypoint),
		GPUs:         strings.TrimSpace(r.config.GPUs),
		Ports:        append([]string{}, r.service.Ports...),
		Env:          env,
		Mounts:       append([]containerengine.Mount{}, r.service.Mounts...),
		SecurityOpts: securityOpts,
		Cmd:          append([]string{}, r.service.Cmd...),
	}, nil
}

func (r *Runtime) waitReady(ctx context.Context) error {
	healthURL := strings.TrimSpace(r.service.HealthURL)
	if healthURL == "" {
		return nil
	}
	deadline := time.Now().Add(2 * time.Minute)
	var lastErr error
	for time.Now().Before(deadline) {
		req, err := newHealthRequest(ctx, healthURL)
		if err != nil {
			return err
		}
		lastErr = probeHealth(req)
		if lastErr == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("backend service not ready: %w", lastErr)
}

func (r *Runtime) containerName() string {
	return "ctgbot-backend-" + safeName(r.registration.Ref())
}

// Native services are retained across bindings; Docker retains its own named state.
type nativeOwners struct {
	enabled  atomic.Bool
	shutdown context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	closed   bool
	services map[string]*nativeRuntime
}

func (f *Factory) bindNative(profile runtimepkg.Profile, config runtimepkg.BindConfig, spec ServiceSpec) (runtimepkg.ServiceRuntime, error) {
	if err := spec.Native.validate(); err != nil {
		return nil, err
	}
	if spec.Identity == "" {
		return nil, fmt.Errorf("native backend requires a stable identity")
	}
	f.owners.mu.Lock()
	defer f.owners.mu.Unlock()
	if f.owners.closed {
		return nil, fmt.Errorf("native backends are closed")
	}
	key := profile.Path + "\x00" + spec.Identity
	if existing := f.owners.services[key]; existing != nil {
		if !reflect.DeepEqual(existing.spec, spec) || !reflect.DeepEqual(existing.env, config.Env) {
			return nil, fmt.Errorf("native backend configuration changed; restart ctgbot before rebinding %s", spec.Identity)
		}
		return existing, nil
	}
	// Freeze caller-owned slices/config so later mutation cannot retarget the child.
	native := *spec.Native
	native.Args = slices.Clone(native.Args)
	spec.Native = &native
	spec.Cmd = slices.Clone(spec.Cmd)
	spec.Env = slices.Clone(spec.Env)
	r := &nativeRuntime{shutdown: f.owners.shutdown, authority: &f.owners.enabled, profile: profile, spec: spec, env: slices.Clone(config.Env)}
	f.owners.services[key] = r
	return r, nil
}

// Close owns only native children. Docker keep-running semantics are unchanged.
func (f *Factory) Close(ctx context.Context) error {
	f.owners.mu.Lock()
	f.owners.closed = true
	f.owners.cancel()
	services := make([]*nativeRuntime, 0, len(f.owners.services))
	for _, r := range f.owners.services {
		services = append(services, r)
	}
	f.owners.mu.Unlock()
	errs := make([]error, len(services))
	var wg sync.WaitGroup
	for i, r := range services {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.mu.Lock()
			defer r.mu.Unlock()
			r.closed = true
			errs[i] = r.stopLocked(ctx)
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

// EnableNative is reserved for the resident run owner, which registers cleanup.
// One-shot CLI factories intentionally cannot spawn or claim native services.
func (f *Factory) EnableNative() error {
	f.owners.mu.Lock()
	defer f.owners.mu.Unlock()
	if f.owners.closed {
		return fmt.Errorf("native backends are closed")
	}
	f.owners.enabled.Store(true)
	return nil
}
