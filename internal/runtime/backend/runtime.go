package backend

import (
	"context"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/containerengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	v5runtime "github.com/bartdeboer/ctgbot/internal/runtime"
)

const Kind = "backend"

type Factory struct {
	componentsRoot string
	logger         *log.Logger
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
	v5runtime.Factory
	BindBackend(registration coremodel.Component, home v5runtime.Home, config v5runtime.BindConfig, service ServiceSpec) *Runtime
}

func New(componentsRoot string, logger *log.Logger) *Factory {
	return &Factory{
		componentsRoot: strings.TrimSpace(componentsRoot),
		logger:         logger,
	}
}

func (f *Factory) Kind() string {
	return Kind
}

func (f *Factory) ComponentHome(registration coremodel.Component) v5runtime.Home {
	hostPath := strings.TrimSpace(registration.HomePath)
	if hostPath == "" {
		hostPath = filepath.Join(f.componentsRoot, registration.Type, registration.Name)
	}
	return v5runtime.Home{Path: hostPath}
}

func (f *Factory) RuntimeComponentHomePath(registration coremodel.Component, home v5runtime.Home) string {
	_, _ = registration, home
	return strings.TrimSpace(home.Path)
}

func (f *Factory) RuntimeWorkspacePath(workspacePath string) string {
	return strings.TrimSpace(workspacePath)
}

func (f *Factory) Bind(
	registration coremodel.Component,
	home v5runtime.Home,
	config v5runtime.BindConfig,
) v5runtime.Runtime {
	return &unsupportedRuntime{
		registration: registration,
		home:         home,
		config:       config.Clean(),
	}
}

func (f *Factory) BindBackend(
	registration coremodel.Component,
	home v5runtime.Home,
	config v5runtime.BindConfig,
	service ServiceSpec,
) *Runtime {
	return &Runtime{
		registration: registration,
		home:         home,
		config:       config.Clean(),
		service:      service.clean(),
		containers:   containerengine.NewManager(f.logger),
	}
}

type Runtime struct {
	registration coremodel.Component
	home         v5runtime.Home
	config       v5runtime.BindConfig
	service      ServiceSpec
	containers   *containerengine.Manager
}

func (r *Runtime) ComponentHome() v5runtime.Home {
	if r == nil {
		return v5runtime.Home{}
	}
	return r.home
}

func (r *Runtime) BaseURL() string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.service.BaseURL)
}

func (r *Runtime) Start(ctx context.Context) (v5runtime.Status, error) {
	container := r.container()
	state, err := container.InspectState(ctx)
	if err != nil {
		return v5runtime.Status{}, err
	}
	if state == containerengine.StateMissing {
		if _, err := r.containers.Create(ctx, r.containerSpec()); err != nil {
			return v5runtime.Status{}, err
		}
		state = containerengine.StateCreated
	}
	if state != containerengine.StateRunning {
		if err := container.Start(ctx); err != nil {
			return v5runtime.Status{}, err
		}
	}
	if err := r.waitReady(ctx); err != nil {
		return v5runtime.Status{}, err
	}
	return r.Status(ctx)
}

func (r *Runtime) Stop(ctx context.Context) error {
	return r.container().Stop(ctx)
}

func (r *Runtime) Refresh(ctx context.Context) error {
	return r.container().Remove(ctx)
}

func (r *Runtime) Status(ctx context.Context) (v5runtime.Status, error) {
	state, err := r.container().InspectState(ctx)
	if err != nil {
		return v5runtime.Status{}, err
	}
	return v5runtime.Status{
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

func (r *Runtime) containerSpec() containerengine.ContainerSpec {
	env := append([]string{}, r.config.Env...)
	env = append(env, r.service.Env...)
	return containerengine.ContainerSpec{
		Name:   r.containerName(),
		Image:  strings.TrimSpace(r.config.Image),
		GPUs:   strings.TrimSpace(r.config.GPUs),
		Ports:  append([]string{}, r.service.Ports...),
		Env:    env,
		Mounts: append([]containerengine.Mount{}, r.service.Mounts...),
		Cmd:    append([]string{}, r.service.Cmd...),
	}
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
	return "ctgbot-v5-backend-" + safeName(r.registration.Ref())
}

type unsupportedRuntime struct {
	registration coremodel.Component
	home         v5runtime.Home
	config       v5runtime.BindConfig
}

func (r *unsupportedRuntime) Kind() string { return Kind }

func (r *unsupportedRuntime) ComponentHome() v5runtime.Home { return r.home }

func (r *unsupportedRuntime) RuntimeComponentHomePath() string {
	return strings.TrimSpace(r.home.Path)
}

func (r *unsupportedRuntime) RuntimeWorkspacePath(workspacePath string) string {
	return strings.TrimSpace(workspacePath)
}

func (r *unsupportedRuntime) Refresh(ctx context.Context, workspacePath string, threadID modeluuid.UUID) error {
	_, _, _ = ctx, workspacePath, threadID
	return fmt.Errorf("backend runtime does not support thread sandbox refresh")
}

func (r *unsupportedRuntime) Start(ctx context.Context, workspacePath string, threadID modeluuid.UUID) (v5runtime.Status, error) {
	_, _, _ = ctx, workspacePath, threadID
	return v5runtime.Status{}, fmt.Errorf("backend runtime does not support thread sandbox start")
}

func (r *unsupportedRuntime) Stop(ctx context.Context, workspacePath string, threadID modeluuid.UUID) error {
	_, _, _ = ctx, workspacePath, threadID
	return fmt.Errorf("backend runtime does not support thread sandbox stop")
}

func (r *unsupportedRuntime) Interrupt(ctx context.Context, workspacePath string, threadID modeluuid.UUID) (bool, error) {
	_, _, _ = ctx, workspacePath, threadID
	return false, fmt.Errorf("backend runtime does not support thread sandbox interrupt")
}

func (r *unsupportedRuntime) Status(ctx context.Context, workspacePath string, threadID modeluuid.UUID) (v5runtime.Status, error) {
	_, _, _ = ctx, workspacePath, threadID
	return v5runtime.Status{}, fmt.Errorf("backend runtime does not support thread sandbox status")
}

func (r *unsupportedRuntime) Exec(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
	stdout io.Writer,
	stderr io.Writer,
	name string,
	args ...string,
) error {
	_, _, _, _, _, _, _, _, _ = ctx, workspacePath, threadID, commands, stdout, stderr, name, args, r.config
	return fmt.Errorf("backend runtime does not support thread sandbox exec")
}

func (r *unsupportedRuntime) CombinedOutput(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
	name string,
	args ...string,
) ([]byte, error) {
	_, _, _, _, _, _, _ = ctx, workspacePath, threadID, commands, name, args, r.config
	return nil, fmt.Errorf("backend runtime does not support thread sandbox combined output")
}

func (r *unsupportedRuntime) OpenHTTPRelayPort(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
	callbackPort int,
	callbackTimeout time.Duration,
) (func(context.Context) error, error) {
	_, _, _, _, _, _, _ = ctx, workspacePath, threadID, commands, callbackPort, callbackTimeout, r.config
	return nil, fmt.Errorf("backend runtime does not support thread sandbox relay ports")
}
