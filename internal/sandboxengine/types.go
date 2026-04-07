package sandboxengine

import (
	"context"
	"os/exec"
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

type Sandbox struct {
	Name string

	WorkspaceDir string
	ProfileDir   string

	ContainerWorkspace    string
	ContainerHome         string
	DeveloperInstructions string

	Hostname     string
	Image        string
	Workdir      string
	Labels       map[string]string
	Env          []string
	Mounts       []Mount
	SecurityOpts []string
	AddHosts     []string
	Cmd          []string

	runtime runtime
}

type Manager interface {
	NewSandbox(name string) *Sandbox
}

type runtime interface {
	ensure(ctx context.Context, sbx *Sandbox) error
	stop(ctx context.Context, sbx *Sandbox) error
	remove(ctx context.Context, sbx *Sandbox) error
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
