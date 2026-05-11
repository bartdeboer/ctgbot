package sandboxengine

import (
	"context"
	"io"
	"sync"

	"github.com/bartdeboer/ctgbot/internal/containerengine"
)

type State string

const (
	StateMissing State = ""
	StateCreated State = "created"
	StateRunning State = "running"
	StateExited  State = "exited"
)

type EnsureAction string

const (
	EnsureNoop    EnsureAction = ""
	EnsureCreated EnsureAction = "created"
	EnsureStarted EnsureAction = "started"
)

type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

type SandboxSpec struct {
	Name string

	InteractiveInterruptEnabled bool

	WorkspaceDir string
	ProfileDir   string

	ContainerWorkspace    string
	ContainerHome         string
	DeveloperInstructions string

	Hostname     string
	Image        string
	Workdir      string
	UserMode     string
	GPUs         string
	Labels       map[string]string
	Env          []string
	Mounts       []Mount
	SecurityOpts []string
	AddHosts     []string
	Cmd          []string
}

type Sandbox struct {
	SandboxSpec

	manager *SandboxManager

	mu                 sync.Mutex
	activeCommand      *SandboxCommand
	activeCommandToken uint64
	container          *containerengine.Container
}

type SandboxCommand struct {
	Name string
	Args []string
}

type Manager interface {
	CreateSandbox(spec *SandboxSpec) *Sandbox
}

func (s *Sandbox) ApplySpec(spec *SandboxSpec) {
	if s == nil || spec == nil {
		return
	}
	s.SandboxSpec = *spec
}

func (s *Sandbox) ContainerSpec() containerengine.ContainerSpec {
	mounts := make([]containerengine.Mount, 0, len(s.Mounts))
	for _, mount := range s.Mounts {
		mounts = append(mounts, containerengine.Mount{
			Source:   mount.Source,
			Target:   mount.Target,
			ReadOnly: mount.ReadOnly,
		})
	}
	return containerengine.ContainerSpec{
		Name:         s.Name,
		Hostname:     s.Hostname,
		Image:        s.Image,
		Workdir:      s.Workdir,
		UserMode:     s.UserMode,
		GPUs:         s.GPUs,
		Labels:       s.Labels,
		Env:          s.Env,
		Mounts:       mounts,
		SecurityOpts: s.SecurityOpts,
		AddHosts:     s.AddHosts,
		Cmd:          s.Cmd,
	}
}

func (s *Sandbox) ensureContainer() *containerengine.Container {
	if s == nil || s.manager == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.container == nil || s.container.Name != s.Name {
		s.container = s.manager.containerManager().Container(s.Name)
	}
	s.container.ApplySpec(s.ContainerSpec())
	return s.container
}

func (s *Sandbox) containerInstance() *containerengine.Container {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.container
}

func (s *Sandbox) setContainer(container *containerengine.Container) {
	if s == nil || container == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.container = container
}

func (s *Sandbox) execOptions(stdout io.Writer, stderr io.Writer) containerengine.ExecOptions {
	if s == nil {
		return containerengine.ExecOptions{Stdout: stdout, Stderr: stderr}
	}
	return containerengine.ExecOptions{
		Env:     append([]string(nil), s.Env...),
		Workdir: s.Workdir,
		Stdout:  stdout,
		Stderr:  stderr,
	}
}

func (s *Sandbox) beginCommand(name string, args ...string) uint64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeCommandToken++
	token := s.activeCommandToken
	s.activeCommand = &SandboxCommand{
		Name: name,
		Args: append([]string(nil), args...),
	}
	return token
}

func (s *Sandbox) endCommand(token uint64) {
	if s == nil || token == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeCommandToken != token {
		return
	}
	s.activeCommand = nil
}

func (s *Sandbox) ActiveCommand() (SandboxCommand, bool) {
	if s == nil {
		return SandboxCommand{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeCommand == nil {
		return SandboxCommand{}, false
	}
	return SandboxCommand{
		Name: s.activeCommand.Name,
		Args: append([]string(nil), s.activeCommand.Args...),
	}, true
}

func (s *Sandbox) Ensure(ctx context.Context) (EnsureAction, error) {
	if s == nil || s.manager == nil {
		return EnsureNoop, nil
	}
	return s.manager.ensure(ctx, s)
}

func (s *Sandbox) InspectState(ctx context.Context) (State, error) {
	if s == nil || s.manager == nil {
		return StateMissing, nil
	}
	if s.Name == "" {
		return StateMissing, nil
	}
	state, err := s.manager.containerManager().InspectState(ctx, s.Name)
	return State(state), err
}

func (s *Sandbox) Stop(ctx context.Context) error {
	if s == nil || s.manager == nil {
		return nil
	}
	return s.manager.stop(ctx, s)
}

func (s *Sandbox) Remove(ctx context.Context) error {
	if s == nil || s.manager == nil {
		return nil
	}
	return s.manager.remove(ctx, s)
}

func (s *Sandbox) Exec(ctx context.Context, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	if s == nil || s.manager == nil {
		return nil
	}
	return s.manager.exec(ctx, s, stdout, stderr, name, args...)
}

func (s *Sandbox) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	if s == nil || s.manager == nil {
		return nil, nil
	}
	return s.manager.combinedOutput(ctx, s, name, args...)
}

func (s *Sandbox) Interrupt() error {
	if s == nil {
		return nil
	}
	container := s.containerInstance()
	if container == nil {
		return nil
	}
	return container.Interrupt()
}

func (s *Sandbox) Interrupted() bool {
	if s == nil {
		return false
	}
	container := s.containerInstance()
	if container == nil {
		return false
	}
	return container.Interrupted()
}
