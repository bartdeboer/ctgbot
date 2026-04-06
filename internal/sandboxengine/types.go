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

type Spec struct {
	Name         string
	Hostname     string
	Image        string
	Workdir      string
	Labels       map[string]string
	Env          []string
	Mounts       []Mount
	SecurityOpts []string
	AddHosts     []string
	Cmd          []string
}

type Sandbox interface {
	CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd
}

type Manager interface {
	InspectState(ctx context.Context, name string) (State, error)
	Ensure(ctx context.Context, spec Spec) (Sandbox, bool, error)
	Stop(ctx context.Context, name string) error
	Remove(ctx context.Context, name string) error
}
