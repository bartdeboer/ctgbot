package containerengine

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
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
	Env         []string
	Workdir     string
	Stdout      io.Writer
	Stderr      io.Writer
	Interactive bool
}

type Container struct {
	ContainerSpec
	manager *Manager

	mu          sync.Mutex
	activeStdin io.WriteCloser
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
	if !opts.Interactive {
		return cmd.Run()
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	c.setActiveStdin(stdin)
	defer c.clearActiveStdin(stdin)
	defer stdin.Close()
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func (c *Container) CombinedOutput(ctx context.Context, opts ExecOptions, name string, args ...string) ([]byte, error) {
	return c.CommandContext(ctx, opts, name, args...).CombinedOutput()
}

func (c *Container) CommandContext(ctx context.Context, opts ExecOptions, name string, args ...string) *exec.Cmd {
	dockerArgs := []string{"exec"}
	if opts.Interactive {
		dockerArgs = append(dockerArgs, "-i")
	}
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

func (c *Container) Interrupt() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	stdin := c.activeStdin
	c.mu.Unlock()
	if stdin == nil {
		return nil
	}
	_, err := stdin.Write([]byte{3})
	if isBenignInterruptWriteError(err) {
		return nil
	}
	return err
}

func (c *Container) setActiveStdin(stdin io.WriteCloser) {
	if c == nil || stdin == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.activeStdin = stdin
}

func (c *Container) clearActiveStdin(stdin io.WriteCloser) {
	if c == nil || stdin == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.activeStdin != stdin {
		return
	}
	c.activeStdin = nil
}

func isBenignInterruptWriteError(err error) bool {
	if err == nil || errors.Is(err, os.ErrClosed) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "closed") || strings.Contains(text, "broken pipe")
}
