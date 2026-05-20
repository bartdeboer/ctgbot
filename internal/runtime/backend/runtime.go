package backend

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
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
}

type ServiceSpec struct {
	BaseURL   string
	HealthURL string
	Ports     []string
	Env       []string
	Mounts    []containerengine.Mount
	Cmd       []string
}

type Binder interface {
	runtimepkg.Factory
	BindBackend(registration coremodel.Component, home runtimepkg.Home, config runtimepkg.BindConfig, service ServiceSpec) *Runtime
}

func New(componentsRoot string, logger *log.Logger) *Factory {
	return &Factory{
		componentsRoot: strings.TrimSpace(componentsRoot),
		logger:         logger,
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

func (f *Factory) ComponentHome(registration coremodel.Component) runtimepkg.Home {
	hostPath := strings.TrimSpace(registration.HomePath)
	if hostPath == "" {
		hostPath = filepath.Join(f.componentsRoot, registration.Type, registration.Name)
	}
	return runtimepkg.Home{Path: hostPath}
}

func (f *Factory) RuntimeComponentHomePath(registration coremodel.Component, home runtimepkg.Home) string {
	_, _ = registration, home
	return strings.TrimSpace(home.Path)
}

func (f *Factory) RuntimeWorkspacePath(workspacePath string) string {
	return strings.TrimSpace(workspacePath)
}

func (f *Factory) BindBackend(
	registration coremodel.Component,
	home runtimepkg.Home,
	config runtimepkg.BindConfig,
	service ServiceSpec,
) *Runtime {
	config = config.WithEnvOverride(f.env...)
	return &Runtime{
		registration: registration,
		home:         home,
		config:       config,
		service:      service.clean(),
		containers:   containerengine.NewManager(f.logger),
	}
}

type Runtime struct {
	registration coremodel.Component
	home         runtimepkg.Home
	config       runtimepkg.BindConfig
	service      ServiceSpec
	containers   *containerengine.Manager
}

func (r *Runtime) ComponentHome() runtimepkg.Home {
	if r == nil {
		return runtimepkg.Home{}
	}
	return r.home
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
		Name:            r.containerName(),
		State:           string(state),
		RuntimeHomePath: strings.TrimSpace(r.home.Path),
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
