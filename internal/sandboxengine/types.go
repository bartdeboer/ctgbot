package sandboxengine

import (
	"context"
	"io"
	"os/exec"
	"sync"
)

type State string

const (
	StateMissing State = ""
	StateCreated State = "created"
	StateRunning State = "running"
	StateExited  State = "exited"
)

type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

type ImageBuilder interface {
	EnsureImage(ctx context.Context) error
	Build(ctx context.Context, noCache bool) error
}

type SandboxSpec struct {
	Name string

	WorkspaceDir string
	ProfileDir   string

	ContainerWorkspace    string
	ContainerHome         string
	DeveloperInstructions string

	Hostname     string
	Image        string
	Workdir      string
	GPUs         string
	Labels       map[string]string
	Env          []string
	Mounts       []Mount
	SecurityOpts []string
	AddHosts     []string
	Cmd          []string
	ImageBuilder ImageBuilder
}

type Sandbox struct {
	SandboxSpec

	runtime runtime

	mu            sync.Mutex
	activeCommand *SandboxCommand
}

type SandboxCommand struct {
	Name string
	Args []string
	cmd  *exec.Cmd
}

type Manager interface {
	CreateSandbox(spec *SandboxSpec) *Sandbox
}

type runtime interface {
	ensure(ctx context.Context, sbx *Sandbox) error
	stop(ctx context.Context, sbx *Sandbox) error
	remove(ctx context.Context, sbx *Sandbox) error
	exec(ctx context.Context, sbx *Sandbox, stdout io.Writer, stderr io.Writer, name string, args ...string) error
	combinedOutput(ctx context.Context, sbx *Sandbox, name string, args ...string) ([]byte, error)
}

func (s *Sandbox) ApplySpec(spec *SandboxSpec) {
	if s == nil || spec == nil {
		return
	}
	s.SandboxSpec = *spec
}

func (s *Sandbox) setActiveCommand(cmd *exec.Cmd, name string, args ...string) {
	if s == nil || cmd == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeCommand = &SandboxCommand{
		Name: name,
		Args: append([]string(nil), args...),
		cmd:  cmd,
	}
}

func (s *Sandbox) clearActiveCommand(cmd *exec.Cmd) {
	if s == nil || cmd == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeCommand == nil || s.activeCommand.cmd != cmd {
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

func (s *Sandbox) Ensure(ctx context.Context) error {
	if s == nil || s.runtime == nil {
		return nil
	}
	return s.runtime.ensure(ctx, s)
}

func (s *Sandbox) Stop(ctx context.Context) error {
	if s == nil || s.runtime == nil {
		return nil
	}
	return s.runtime.stop(ctx, s)
}

func (s *Sandbox) Remove(ctx context.Context) error {
	if s == nil || s.runtime == nil {
		return nil
	}
	return s.runtime.remove(ctx, s)
}

func (s *Sandbox) Exec(ctx context.Context, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	if s == nil || s.runtime == nil {
		return nil
	}
	return s.runtime.exec(ctx, s, stdout, stderr, name, args...)
}

func (s *Sandbox) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	if s == nil || s.runtime == nil {
		return nil, nil
	}
	return s.runtime.combinedOutput(ctx, s, name, args...)
}

func (s *Sandbox) CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	dockerArgs := []string{"exec"}
	for _, env := range s.Env {
		if env == "" {
			continue
		}
		dockerArgs = append(dockerArgs, "-e", env)
	}
	if s.Workdir != "" {
		dockerArgs = append(dockerArgs, "-w", s.Workdir)
	}
	dockerArgs = append(dockerArgs, s.Name, name)
	dockerArgs = append(dockerArgs, args...)
	return exec.CommandContext(ctx, "docker", dockerArgs...)
}
