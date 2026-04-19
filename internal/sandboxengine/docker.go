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
	mu         sync.Mutex
	Containers *containerengine.Manager
	Logger     *log.Logger
	locks      *sandboxLocks
	sandboxes  map[string]*Sandbox
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
		Containers: containerengine.NewManager(logger),
		locks:      &sandboxLocks{locks: map[string]*sandboxLock{}},
		sandboxes:  map[string]*Sandbox{},
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
	copySpec := *spec
	copySpec.Name = strings.TrimSpace(copySpec.Name)

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sandboxes == nil {
		m.sandboxes = map[string]*Sandbox{}
	}
	if sbx := m.sandboxes[copySpec.Name]; sbx != nil {
		sbx.ApplySpec(&copySpec)
		sbx.docker = m
		return sbx
	}
	sbx := &Sandbox{docker: m}
	sbx.ApplySpec(&copySpec)
	m.sandboxes[copySpec.Name] = sbx
	return sbx
}

func (m *DockerManager) ensure(ctx context.Context, sbx *Sandbox) error {
	if sbx == nil || strings.TrimSpace(sbx.Name) == "" {
		return fmt.Errorf("missing sandbox name")
	}
	return m.withLock(sbx.Name, func() error {
		return sbx.ensureReady(ctx)
	})
}

func (s *Sandbox) ensureReady(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("missing sandbox")
	}
	if s.ImageBuilder != nil {
		if err := s.ImageBuilder.EnsureImage(ctx); err != nil {
			return err
		}
	}
	container := s.ensureContainer()
	if container == nil {
		return fmt.Errorf("missing backing container")
	}
	state, err := container.InspectState(ctx)
	if err != nil {
		return err
	}
	switch State(state) {
	case StateRunning:
		return nil
	case StateCreated, StateExited:
		return container.Start(ctx)
	case StateMissing:
		container, err := s.docker.containerManager().Create(ctx, s.ContainerSpec())
		if err != nil {
			return err
		}
		s.setContainer(container)
		return container.Start(ctx)
	default:
		return fmt.Errorf("unsupported sandbox state %q for %s", state, s.Name)
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
		if err := sbx.ensureReady(ctx); err != nil {
			return err
		}
		container := sbx.ensureContainer()
		if container == nil {
			return fmt.Errorf("missing backing container")
		}
		token := sbx.beginCommand(name, args...)
		defer sbx.endCommand(token)
		opts := sbx.execOptions(stdout, stderr)
		opts.Interactive = true
		return container.Exec(ctx, opts, name, args...)
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
		if err := sbx.ensureReady(ctx); err != nil {
			return err
		}
		container := sbx.ensureContainer()
		if container == nil {
			return fmt.Errorf("missing backing container")
		}
		buf, err := container.CombinedOutput(ctx, sbx.execOptions(nil, nil), name, args...)
		out = append([]byte(nil), buf...)
		return err
	})
	return out, err
}

func (m *DockerManager) containerManager() *containerengine.Manager {
	return m.Containers
}
