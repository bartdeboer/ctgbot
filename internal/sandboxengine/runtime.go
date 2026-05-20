package sandboxengine

import (
	"context"
	"io"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
)

type CommandExecutor = commandengine.CommandExecutor

type RuntimeManager interface {
	Manager
	CreateRuntime(spec RuntimeSpec) *SandboxRuntime
	BeginSession(ctx context.Context, spec SandboxSpec, options SessionOptions) (*Session, error)
}

type Provider interface {
	SandboxManager() RuntimeManager
}

type RuntimeSpec struct {
	Sandbox       SandboxSpec
	AgentCommands CommandExecutor
}

type SandboxRuntime struct {
	sandbox       *Sandbox
	agentCommands CommandExecutor
}

func (m *SandboxManager) CreateRuntime(spec RuntimeSpec) *SandboxRuntime {
	runtime := &SandboxRuntime{
		sandbox: m.CreateSandbox(&spec.Sandbox),
	}
	runtime.SetAgentCommands(spec.AgentCommands)
	return runtime
}

func (r *SandboxRuntime) Sandbox() *Sandbox {
	if r == nil {
		return nil
	}
	return r.sandbox
}

func (r *SandboxRuntime) Exec(ctx context.Context, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	if r == nil || r.sandbox == nil {
		return nil
	}
	return r.sandbox.Exec(ctx, stdout, stderr, name, args...)
}

func (r *SandboxRuntime) ExecTTY(ctx context.Context, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	if r == nil || r.sandbox == nil {
		return nil
	}
	return r.sandbox.ExecTTY(ctx, stdout, stderr, name, args...)
}

func (r *SandboxRuntime) Stop(ctx context.Context) error {
	if r == nil || r.sandbox == nil {
		return nil
	}
	return r.sandbox.Stop(ctx)
}

func (r *SandboxRuntime) AgentCommands() CommandExecutor {
	if r == nil {
		return nil
	}
	return r.agentCommands
}

func (r *SandboxRuntime) SetAgentCommands(commands CommandExecutor) {
	if r == nil {
		return
	}
	r.agentCommands = commands
}
