package containerengine

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
)

const ActivePIDFile = "/tmp/ctgbot-codex.pid"

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
	User         string
	UserMode     string
	GPUs         string
	Ports        []string
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
	TTY         bool
}

type Container struct {
	ContainerSpec
	manager *Manager

	mu          sync.Mutex
	interrupted bool
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
	c.clearInterrupted()
	cmd := c.CommandContext(ctx, opts, name, args...)
	if opts.Interactive {
		cmd.Stdin = os.Stdin
	}
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	return cmd.Run()
}

func (c *Container) CombinedOutput(ctx context.Context, opts ExecOptions, name string, args ...string) ([]byte, error) {
	return c.CommandContext(ctx, opts, name, args...).CombinedOutput()
}

func (c *Container) CommandContext(ctx context.Context, opts ExecOptions, name string, args ...string) *exec.Cmd {
	dockerArgs := c.execArgs(opts, name, args...)
	return exec.CommandContext(ctx, "docker", dockerArgs...)
}

func (c *Container) execArgs(opts ExecOptions, name string, args ...string) []string {
	dockerArgs := []string{"exec"}
	if opts.Interactive {
		dockerArgs = append(dockerArgs, "-i")
	}
	if opts.TTY {
		dockerArgs = append(dockerArgs, "-t")
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
	return dockerArgs
}

func (c *Container) Interrupt() error {
	if c == nil || c.manager == nil || c.Name == "" {
		return nil
	}
	c.markInterrupted()
	cmd := c.interruptCommandContext(context.Background())
	out, err := cmd.CombinedOutput()
	if err != nil {
		c.manager.logf("interrupt attempt finished name=%s err=%v out=%s", c.Name, err, string(out))
		return nil
	}
	return nil
}

func (c *Container) interruptCommandContext(ctx context.Context) *exec.Cmd {
	script := "test -s " + ActivePIDFile + " && kill -INT $(cat " + ActivePIDFile + ") 2>/dev/null || true"
	return exec.CommandContext(ctx, "docker", "exec", c.Name, "sh", "-lc", script)
}

func (c *Container) Interrupted() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.interrupted
}

func (c *Container) markInterrupted() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.interrupted = true
}

func (c *Container) clearInterrupted() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.interrupted = false
}
