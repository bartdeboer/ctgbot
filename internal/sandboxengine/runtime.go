package sandboxengine

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
)

type CommandExecutor interface {
	Execute(ctx context.Context, req commandengine.Request) (commandengine.Result, error)
}

type RuntimeSpec struct {
	Sandbox  SandboxSpec
	Commands CommandExecutor
}

type Runtime struct {
	sandbox  *Sandbox
	commands CommandExecutor
}

func (m *SandboxManager) CreateRuntime(spec RuntimeSpec) *Runtime {
	return &Runtime{
		sandbox:  m.CreateSandbox(&spec.Sandbox),
		commands: spec.Commands,
	}
}

func (r *Runtime) Sandbox() *Sandbox {
	if r == nil {
		return nil
	}
	return r.sandbox
}

func (r *Runtime) Commands() CommandExecutor {
	if r == nil {
		return nil
	}
	return r.commands
}
