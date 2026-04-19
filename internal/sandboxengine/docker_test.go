package sandboxengine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/containerengine"
)

func TestSandboxExecDelegatesToRuntime(t *testing.T) {
	t.Parallel()
	rt := &fakeSandboxRuntime{}
	sbx := &Sandbox{SandboxSpec: SandboxSpec{Name: "ctgbot-test"}, runtime: rt}
	var stdout bytes.Buffer
	if err := sbx.Exec(context.Background(), &stdout, nil, "codex", "exec", "hello"); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if len(rt.execCalls) != 1 {
		t.Fatalf("exec calls = %d, want 1", len(rt.execCalls))
	}
	if rt.execCalls[0].name != "codex" {
		t.Fatalf("name = %q", rt.execCalls[0].name)
	}
	if stdout.String() != "out" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSandboxCombinedOutputDelegatesToRuntime(t *testing.T) {
	t.Parallel()
	rt := &fakeSandboxRuntime{combinedOut: []byte("hello")}
	sbx := &Sandbox{SandboxSpec: SandboxSpec{Name: "ctgbot-test"}, runtime: rt}
	out, err := sbx.CombinedOutput(context.Background(), "cat", "/tmp/file")
	if err != nil {
		t.Fatalf("CombinedOutput: %v", err)
	}
	if string(out) != "hello" {
		t.Fatalf("out = %q", string(out))
	}
}

func TestSandboxLocksAcquireRelease(t *testing.T) {
	t.Parallel()
	locks := &sandboxLocks{locks: map[string]*sandboxLock{}}
	lock := locks.acquire("sbx-1")
	if lock == nil {
		t.Fatalf("expected lock")
	}
	locks.release("sbx-1", lock)
	if len(locks.locks) != 0 {
		t.Fatalf("locks map = %#v", locks.locks)
	}
}

type fakeSandboxRuntime struct {
	execCalls []struct {
		name string
		args []string
	}
	combinedOut []byte
}

func (f *fakeSandboxRuntime) ensure(ctx context.Context, sbx *Sandbox) error { return nil }
func (f *fakeSandboxRuntime) stop(ctx context.Context, sbx *Sandbox) error   { return nil }
func (f *fakeSandboxRuntime) remove(ctx context.Context, sbx *Sandbox) error { return nil }
func (f *fakeSandboxRuntime) exec(ctx context.Context, sbx *Sandbox, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	f.execCalls = append(f.execCalls, struct {
		name string
		args []string
	}{name: name, args: append([]string(nil), args...)})
	if stdout != nil {
		_, _ = stdout.Write([]byte("out"))
	}
	return nil
}
func (f *fakeSandboxRuntime) combinedOutput(ctx context.Context, sbx *Sandbox, name string, args ...string) ([]byte, error) {
	return append([]byte(nil), f.combinedOut...), nil
}

type fakeImageBuilder struct {
	ensureCalls int
	ensureErr   error
}

func (f *fakeImageBuilder) EnsureImage(ctx context.Context) error {
	f.ensureCalls++
	return f.ensureErr
}

func (f *fakeImageBuilder) Build(ctx context.Context, noCache bool) error { return nil }

func TestBuilderSetsImageBuilder(t *testing.T) {
	t.Parallel()
	img := &fakeImageBuilder{}
	spec := NewBuilder("ctgbot-test").ImageBuilder(img).Build()
	if spec.ImageBuilder != img {
		t.Fatalf("image builder not attached")
	}
}

func TestDockerManagerExecEnsuresImageFirst(t *testing.T) {
	t.Parallel()
	img := &fakeImageBuilder{ensureErr: fmt.Errorf("boom")}
	mgr := NewSandboxManager(nil)
	sbx := mgr.CreateSandbox(&SandboxSpec{Name: "ctgbot-test", ImageBuilder: img})
	if err := sbx.Exec(context.Background(), nil, nil, "codex", "exec"); err == nil {
		t.Fatalf("expected ensure image error")
	}
	if img.ensureCalls != 1 {
		t.Fatalf("ensure calls = %d, want 1", img.ensureCalls)
	}
}

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
	mgr := containerengine.NewManager(nil)
	sbx := &Sandbox{SandboxSpec: SandboxSpec{Name: "ctgbot-test", Image: "ctgbot:latest"}}
	first := sbx.ensureContainer(mgr)
	second := sbx.ensureContainer(mgr)
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
