package containerengine

import (
	"context"
	"io"
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

type ContainerSpec struct {
	Name         string
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
}

type ExecOptions struct {
	Env     []string
	Workdir string
	Stdout  io.Writer
	Stderr  io.Writer
}

type Container struct {
	ContainerSpec
	manager *Manager
}

func (c *Container) ApplySpec(spec ContainerSpec) {
	c.ContainerSpec = spec
}

func (c *Container) InspectState(ctx context.Context) (State, error) {
	if c == nil || c.manager == nil {
		return StateMissing, nil
	}
	return c.manager.inspectState(ctx, c.Name)
}

func (c *Container) Start(ctx context.Context) error {
	if c == nil || c.manager == nil {
		return nil
	}
	return c.manager.start(ctx, c.Name)
}

func (c *Container) Stop(ctx context.Context) error {
	if c == nil || c.manager == nil {
		return nil
	}
	return c.manager.stop(ctx, c.Name)
}

func (c *Container) Remove(ctx context.Context) error {
	if c == nil || c.manager == nil {
		return nil
	}
	return c.manager.remove(ctx, c.Name)
}

func (c *Container) Exec(ctx context.Context, opts ExecOptions, name string, args ...string) error {
	cmd := c.CommandContext(ctx, opts, name, args...)
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	return cmd.Run()
}

func (c *Container) CombinedOutput(ctx context.Context, opts ExecOptions, name string, args ...string) ([]byte, error) {
	return c.CommandContext(ctx, opts, name, args...).CombinedOutput()
}

func (c *Container) CommandContext(ctx context.Context, opts ExecOptions, name string, args ...string) *exec.Cmd {
	dockerArgs := []string{"exec"}
	for _, env := range opts.Env {
		if env == "" {
			continue
		}
		dockerArgs = append(dockerArgs, "-e", env)
	}
	if opts.Workdir != "" {
		dockerArgs = append(dockerArgs, "-w", opts.Workdir)
	}
	dockerArgs = append(dockerArgs, c.Name, name)
	dockerArgs = append(dockerArgs, args...)
	return exec.CommandContext(ctx, "docker", dockerArgs...)
}
