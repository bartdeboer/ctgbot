package containerengine

import (
	"bytes"
	"context"
	"io"
	"testing"
)

func TestIsMissingContainerOutputIsCaseInsensitive(t *testing.T) {
	t.Parallel()

	cases := []string{
		"Error response from daemon: No such object: ctgbot-test",
		"error: no such object: ctgbot-test",
		"Error response from daemon: No such container: ctgbot-test",
	}

	for _, msg := range cases {
		msg := msg
		t.Run(msg, func(t *testing.T) {
			t.Parallel()
			if !isMissingContainerOutput(msg) {
				t.Fatalf("expected %q to be treated as a missing container", msg)
			}
		})
	}
}

func TestBuildCreateArgsIncludesGPUs(t *testing.T) {
	t.Parallel()

	args := buildCreateArgs(ContainerSpec{
		Name:  "ctgbot-test",
		Image: "ctgbot:latest",
		GPUs:  "all",
		Cmd:   []string{"tail", "-f", "/dev/null"},
	})

	if len(args) == 0 {
		t.Fatalf("expected docker create args")
	}

	found := false
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--gpus" && args[i+1] == "all" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected --gpus all in args: %#v", args)
	}
}

func TestManagerRetainsContainerInstancesByName(t *testing.T) {
	t.Parallel()

	mgr := NewManager(nil)
	first := mgr.Container("ctgbot-test")
	second := mgr.Container("ctgbot-test")
	if first != second {
		t.Fatalf("expected same container instance")
	}
}

func TestCreateUpdatesRetainedContainerSpecEvenWhenDockerFails(t *testing.T) {
	t.Parallel()

	mgr := NewManager(nil)
	container, err := mgr.Create(context.Background(), ContainerSpec{
		Name:    "ctgbot-test",
		Image:   "ctgbot:latest",
		Workdir: "/workspace",
	})
	if container == nil {
		t.Fatalf("expected retained container instance")
	}
	if container.Workdir != "/workspace" {
		t.Fatalf("workdir = %q, want /workspace", container.Workdir)
	}
	if err == nil {
		t.Skip("docker is available in this environment; spec-update assertion already passed")
	}
}

func TestContainerCommandContextBuildsDockerExec(t *testing.T) {
	t.Parallel()

	container := &Container{ContainerSpec: ContainerSpec{Name: "ctgbot-test"}}
	cmd := container.CommandContext(context.Background(), ExecOptions{
		Env:     []string{"HOME=/codex-home", "CODEX_HOME=/codex-home"},
		Workdir: "/workspace",
	}, "codex", "exec", "hello")

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

func TestContainerInterruptWritesCtrlC(t *testing.T) {
	t.Parallel()
	buf := &recordingWriteCloser{}
	container := &Container{}
	container.setActiveStdin(buf)
	if err := container.Interrupt(); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	if got := buf.String(); got != string([]byte{3}) {
		t.Fatalf("stdin write = %q, want ctrl-c", got)
	}
}

func TestContainerInterruptIgnoresClosedPipe(t *testing.T) {
	t.Parallel()
	container := &Container{}
	container.setActiveStdin(&errWriteCloser{err: io.ErrClosedPipe})
	if err := container.Interrupt(); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
}

func TestContainerCommandContextInteractiveAddsInputFlag(t *testing.T) {
	t.Parallel()
	container := &Container{ContainerSpec: ContainerSpec{Name: "ctgbot-test"}}
	cmd := container.CommandContext(context.Background(), ExecOptions{Interactive: true}, "codex", "exec")
	got := cmd.Args
	want := []string{"docker", "exec", "-i", "ctgbot-test", "codex", "exec"}
	if len(got) != len(want) {
		t.Fatalf("args len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg[%d] = %q, want %q; all args: %#v", i, got[i], want[i], got)
		}
	}
}

type recordingWriteCloser struct{ bytes.Buffer }

func (r *recordingWriteCloser) Close() error { return nil }

type errWriteCloser struct{ err error }

func (w *errWriteCloser) Write(p []byte) (int, error) { return 0, w.err }
func (w *errWriteCloser) Close() error                { return nil }
