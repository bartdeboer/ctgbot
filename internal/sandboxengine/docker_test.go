package sandboxengine

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/containerengine"
)

func TestNewSandboxManagerInitializesSharedState(t *testing.T) {
	t.Parallel()
	mgr := NewSandboxManager(nil)
	if mgr.Containers == nil {
		t.Fatalf("expected container manager")
	}
	if mgr.locks == nil || mgr.locks.locks == nil {
		t.Fatalf("expected sandbox locks to be initialized")
	}
	if mgr.sandboxes == nil {
		t.Fatalf("expected sandbox registry to be initialized")
	}
}

func TestCreateSandboxReusesInstanceByName(t *testing.T) {
	t.Parallel()
	mgr := NewSandboxManager(nil)
	first := mgr.CreateSandbox(&SandboxSpec{Name: "ctgbot-test", Workdir: "/workspace"})
	second := mgr.CreateSandbox(&SandboxSpec{Name: "ctgbot-test", Workdir: "/repo"})
	if first != second {
		t.Fatalf("expected same sandbox instance")
	}
	if second.Workdir != "/repo" {
		t.Fatalf("workdir = %q, want /repo", second.Workdir)
	}
}

func TestCreateSandboxKeepsDifferentNamesDistinct(t *testing.T) {
	t.Parallel()
	mgr := NewSandboxManager(nil)
	first := mgr.CreateSandbox(&SandboxSpec{Name: "one"})
	second := mgr.CreateSandbox(&SandboxSpec{Name: "two"})
	if first == second {
		t.Fatalf("expected different sandbox instances")
	}
}

func TestSandboxActiveCommandTracking(t *testing.T) {
	t.Parallel()
	sbx := &Sandbox{}
	token := sbx.beginCommand("codex", "exec", "hello")
	active, ok := sbx.ActiveCommand()
	if !ok {
		t.Fatalf("expected active command")
	}
	if active.Name != "codex" {
		t.Fatalf("name = %q, want codex", active.Name)
	}
	if len(active.Args) != 2 || active.Args[0] != "exec" || active.Args[1] != "hello" {
		t.Fatalf("args = %#v", active.Args)
	}
	sbx.endCommand(token)
	if _, ok := sbx.ActiveCommand(); ok {
		t.Fatalf("expected active command to be cleared")
	}
}

func TestSandboxContainerSpecMapsFields(t *testing.T) {
	t.Parallel()
	sbx := &Sandbox{SandboxSpec: SandboxSpec{
		Name:         "ctgbot-test",
		Hostname:     "ctgbot-test",
		Image:        "ctgbot:latest",
		Workdir:      "/workspace",
		GPUs:         "all",
		Labels:       map[string]string{"a": "b"},
		Env:          []string{"HOME=/codex-home"},
		Mounts:       []Mount{{Source: "/src", Target: "/dst", ReadOnly: true}},
		SecurityOpts: []string{"seccomp=unconfined"},
		AddHosts:     []string{"host.docker.internal:host-gateway"},
		Cmd:          []string{"tail", "-f", "/dev/null"},
	}}
	spec := sbx.ContainerSpec()
	if spec.Name != sbx.Name || spec.Image != sbx.Image || spec.Workdir != sbx.Workdir {
		t.Fatalf("unexpected spec: %#v", spec)
	}
	if len(spec.Mounts) != 1 || spec.Mounts[0].Source != "/src" || !spec.Mounts[0].ReadOnly {
		t.Fatalf("mounts = %#v", spec.Mounts)
	}
}

func TestSandboxEnsureContainerStoresResolvedInstance(t *testing.T) {
	t.Parallel()
	docker := NewSandboxManager(nil)
	sbx := &Sandbox{SandboxSpec: SandboxSpec{Name: "ctgbot-test", Image: "ctgbot:latest"}, manager: docker}
	first := sbx.ensureContainer()
	second := sbx.ensureContainer()
	if first == nil || second == nil {
		t.Fatalf("expected resolved container instance")
	}
	if first != second {
		t.Fatalf("expected same resolved container instance")
	}
	if first.Name != "ctgbot-test" || first.Image != "ctgbot:latest" {
		t.Fatalf("unexpected container: %#v", first)
	}
}

func TestSandboxEnsureContainerUsesSandboxManagerContainerRegistry(t *testing.T) {
	t.Parallel()
	docker := NewSandboxManager(nil)
	sbx := &Sandbox{SandboxSpec: SandboxSpec{Name: "ctgbot-test", Image: "ctgbot:latest"}, manager: docker}
	container := sbx.ensureContainer()
	if container != docker.containerManager().Container("ctgbot-test") {
		t.Fatalf("expected sandbox container to come from docker manager registry")
	}
}

func TestSandboxWithoutDockerIsNoop(t *testing.T) {
	t.Parallel()
	sbx := &Sandbox{SandboxSpec: SandboxSpec{Name: "ctgbot-test"}}
	if action, err := sbx.Ensure(context.Background()); err != nil || action != EnsureNoop {
		t.Fatalf("Ensure: %v", err)
	}
	if err := sbx.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := sbx.Remove(context.Background()); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := sbx.Exec(context.Background(), nil, nil, "echo", "hi"); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if out, err := sbx.CombinedOutput(context.Background(), "echo", "hi"); err != nil || out != nil {
		t.Fatalf("CombinedOutput = %q, %v; want nil, nil", string(out), err)
	}
}

var _ = containerengine.StateRunning
