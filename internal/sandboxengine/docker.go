package sandboxengine

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"

	"github.com/bartdeboer/ctgbot/internal/containerengine"
)

type DockerManager struct {
	Containers *containerengine.Manager
	Logger     *log.Logger
	locks      *sandboxLocks
}

type sandboxLocks struct {
	mu    sync.Mutex
	locks map[string]*sandboxLock
}

type sandboxLock struct {
	mu       sync.Mutex
	refCount int
}

func NewSandboxManager(logger *log.Logger) *DockerManager {
	return &DockerManager{
		Logger:     logger,
		Containers: &containerengine.Manager{Logger: logger},
		locks:      &sandboxLocks{locks: map[string]*sandboxLock{}},
	}
}

func (m *DockerManager) ensureLocks() *sandboxLocks {
	return m.locks
}

func (m *DockerManager) withLock(name string, fn func() error) error {
	if strings.TrimSpace(name) == "" {
		if fn == nil {
			return nil
		}
		return fn()
	}
	locks := m.ensureLocks()
	lock := locks.acquire(name)
	defer locks.release(name, lock)
	lock.mu.Lock()
	defer lock.mu.Unlock()
	if fn == nil {
		return nil
	}
	return fn()
}

func (l *sandboxLocks) acquire(name string) *sandboxLock {
	l.mu.Lock()
	defer l.mu.Unlock()
	lock := l.locks[name]
	if lock == nil {
		lock = &sandboxLock{}
		l.locks[name] = lock
	}
	lock.refCount++
	return lock
}

func (l *sandboxLocks) release(name string, lock *sandboxLock) {
	if l == nil || lock == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	current := l.locks[name]
	if current != lock {
		return
	}
	current.refCount--
	if current.refCount <= 0 {
		delete(l.locks, name)
	}
}

func (m *DockerManager) CreateSandbox(spec *SandboxSpec) *Sandbox {
	if spec == nil {
		spec = &SandboxSpec{}
	}
	sbx := &Sandbox{
		SandboxSpec: *spec,
		runtime:     m,
	}
	sbx.Name = strings.TrimSpace(sbx.Name)
	return sbx
}

func (m *DockerManager) ensure(ctx context.Context, sbx *Sandbox) error {
	if sbx == nil || strings.TrimSpace(sbx.Name) == "" {
		return fmt.Errorf("missing sandbox name")
	}
	return m.withLock(sbx.Name, func() error {
		return m.ensureReady(ctx, sbx)
	})
}

func (m *DockerManager) ensureReady(ctx context.Context, sbx *Sandbox) error {
	if sbx != nil && sbx.ImageBuilder != nil {
		if err := sbx.ImageBuilder.EnsureImage(ctx); err != nil {
			return err
		}
	}
	state, err := m.inspectState(ctx, sbx.Name)
	if err != nil {
		return err
	}
	switch state {
	case StateRunning:
		return nil
	case StateCreated, StateExited:
		return m.containerManager().Start(ctx, sbx.Name)
	case StateMissing:
		if err := m.containerManager().Create(ctx, m.toContainerSpec(sbx)); err != nil {
			return err
		}
		return m.containerManager().Start(ctx, sbx.Name)
	default:
		return fmt.Errorf("unsupported sandbox state %q for %s", state, sbx.Name)
	}
}

func (m *DockerManager) stop(ctx context.Context, sbx *Sandbox) error {
	if sbx == nil || strings.TrimSpace(sbx.Name) == "" {
		return nil
	}
	return m.withLock(sbx.Name, func() error {
		return m.containerManager().Stop(ctx, sbx.Name)
	})
}

func (m *DockerManager) remove(ctx context.Context, sbx *Sandbox) error {
	if sbx == nil || strings.TrimSpace(sbx.Name) == "" {
		return nil
	}
	return m.withLock(sbx.Name, func() error {
		return m.containerManager().Remove(ctx, sbx.Name)
	})
}

func (m *DockerManager) exec(ctx context.Context, sbx *Sandbox, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	if sbx == nil || strings.TrimSpace(sbx.Name) == "" {
		return fmt.Errorf("missing sandbox name")
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("missing executable name")
	}
	return m.withLock(sbx.Name, func() error {
		if err := m.ensureReady(ctx, sbx); err != nil {
			return err
		}
		cmd := sbx.CommandContext(ctx, name, args...)
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		return cmd.Run()
	})
}

func (m *DockerManager) combinedOutput(ctx context.Context, sbx *Sandbox, name string, args ...string) ([]byte, error) {
	if sbx == nil || strings.TrimSpace(sbx.Name) == "" {
		return nil, fmt.Errorf("missing sandbox name")
	}
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("missing executable name")
	}
	var out []byte
	err := m.withLock(sbx.Name, func() error {
		if err := m.ensureReady(ctx, sbx); err != nil {
			return err
		}
		cmd := sbx.CommandContext(ctx, name, args...)
		buf, err := cmd.CombinedOutput()
		out = append([]byte(nil), buf...)
		return err
	})
	return out, err
}

func (m *DockerManager) inspectState(ctx context.Context, name string) (State, error) {
	state, err := m.containerManager().InspectState(ctx, name)
	if err != nil {
		return StateMissing, err
	}
	return State(state), nil
}

func (m *DockerManager) containerManager() *containerengine.Manager {
	return m.Containers
}

func (m *DockerManager) toContainerSpec(sbx *Sandbox) containerengine.ContainerSpec {
	mounts := make([]containerengine.Mount, 0, len(sbx.Mounts))
	for _, mount := range sbx.Mounts {
		mounts = append(mounts, containerengine.Mount{
			Source:   mount.Source,
			Target:   mount.Target,
			ReadOnly: mount.ReadOnly,
		})
	}
	return containerengine.ContainerSpec{
		Name:         sbx.Name,
		Hostname:     sbx.Hostname,
		Image:        sbx.Image,
		Workdir:      sbx.Workdir,
		GPUs:         sbx.GPUs,
		Labels:       sbx.Labels,
		Env:          sbx.Env,
		Mounts:       mounts,
		SecurityOpts: sbx.SecurityOpts,
		AddHosts:     sbx.AddHosts,
		Cmd:          sbx.Cmd,
	}
}
