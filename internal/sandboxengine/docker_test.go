package sandboxengine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
)

func TestDockerSandboxCommandContextBuildsDockerExec(t *testing.T) {
	t.Parallel()

	sbx := Sandbox{
		Name:    "ctgbot-test",
		Workdir: "/workspace",
		Env:     []string{"HOME=/codex-home", "CODEX_HOME=/codex-home"},
	}
	cmd := sbx.CommandContext(context.Background(), "codex", "exec", "hello")

	got := cmd.Args
	want := []string{
		"docker",
		"exec",
		"-e", "HOME=/codex-home",
		"-e", "CODEX_HOME=/codex-home",
		"-w", "/workspace",
		"ctgbot-test",
		"codex",
		"exec",
		"hello",
	}
	if len(got) != len(want) {
		t.Fatalf("args len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg[%d] = %q, want %q; all args: %#v", i, got[i], want[i], got)
		}
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

func TestSandboxExecDelegatesToRuntime(t *testing.T) {
	t.Parallel()
	rt := &fakeSandboxRuntime{}
	sbx := &Sandbox{Name: "ctgbot-test", runtime: rt}
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
	sbx := &Sandbox{Name: "ctgbot-test", runtime: rt}
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
	mgr := NewSandboxManager(nil)
	sbx := NewBuilder(mgr, "ctgbot-test").ImageBuilder(img).Build()
	if sbx.ImageBuilder != img {
		t.Fatalf("image builder not attached")
	}
}

func TestDockerManagerExecEnsuresImageFirst(t *testing.T) {
	t.Parallel()
	img := &fakeImageBuilder{ensureErr: fmt.Errorf("boom")}
	mgr := NewSandboxManager(nil)
	sbx := mgr.NewSandbox("ctgbot-test")
	sbx.ImageBuilder = img
	if err := sbx.Exec(context.Background(), nil, nil, "codex", "exec"); err == nil {
		t.Fatalf("expected ensure image error")
	}
	if img.ensureCalls != 1 {
		t.Fatalf("ensure calls = %d, want 1", img.ensureCalls)
	}
}
