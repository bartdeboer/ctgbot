package sandboxengine

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
)

type fakeCommandExecutor struct{}

var _ CommandExecutor = (*fakeCommandExecutor)(nil)

func (f *fakeCommandExecutor) Execute(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	return commandengine.Result{Text: "ok"}, nil
}

func TestCreateRuntimeAttachesSandboxAndCommands(t *testing.T) {
	t.Parallel()

	mgr := NewSandboxManager(nil)
	commands := &fakeCommandExecutor{}
	runtime := mgr.CreateRuntime(RuntimeSpec{
		Sandbox:       SandboxSpec{Name: "ctgbot-runtime-test", Workdir: "/workspace"},
		AgentCommands: commands,
	})

	if runtime == nil {
		t.Fatal("expected runtime")
	}
	if runtime.Sandbox() == nil || runtime.Sandbox().Name != "ctgbot-runtime-test" {
		t.Fatalf("unexpected runtime sandbox: %#v", runtime.Sandbox())
	}
	if runtime.AgentCommands() != commands {
		t.Fatalf("runtime agent commands = %#v, want %#v", runtime.AgentCommands(), commands)
	}

	again := mgr.CreateRuntime(RuntimeSpec{
		Sandbox:       SandboxSpec{Name: "ctgbot-runtime-test", Workdir: "/repo"},
		AgentCommands: commands,
	})
	if again.Sandbox() != runtime.Sandbox() {
		t.Fatal("expected runtime to reuse sandbox by name")
	}
	if again.Sandbox().Workdir != "/repo" {
		t.Fatalf("sandbox workdir = %q, want /repo", again.Sandbox().Workdir)
	}
}

func TestNilRuntimeAccessors(t *testing.T) {
	t.Parallel()

	var runtime *SandboxRuntime
	if runtime.Sandbox() != nil {
		t.Fatal("nil runtime sandbox should be nil")
	}
	if runtime.AgentCommands() != nil {
		t.Fatal("nil runtime agent commands should be nil")
	}
	runtime.SetAgentCommands(&fakeCommandExecutor{})
}

func TestRuntimeAgentCommandSetter(t *testing.T) {
	t.Parallel()

	runtime := &SandboxRuntime{}
	commands := &fakeCommandExecutor{}
	runtime.SetAgentCommands(commands)

	if runtime.AgentCommands() != commands {
		t.Fatalf("runtime agent commands = %#v, want %#v", runtime.AgentCommands(), commands)
	}
}
