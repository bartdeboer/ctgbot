package chatcommandsv2

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type fakeRunner struct {
	requests []Request
}

func (f *fakeRunner) Execute(req Request) error {
	f.requests = append(f.requests, req)
	return nil
}

func withStdin(t *testing.T, data string, fn func()) {
	t.Helper()
	old := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	if _, err := w.WriteString(data); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	os.Stdin = r
	defer func() {
		os.Stdin = old
		_ = r.Close()
	}()
	fn()
}

func TestRunBuildsRunCommandRequest(t *testing.T) {
	runner := &fakeRunner{}
	cmds := New(runner)

	withStdin(t, "hello", func() {
		if err := cmds.Run(context.Background(), []string{"run", "echo", "a", "b"}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})

	if len(runner.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(runner.requests))
	}
	got := runner.requests[0]
	if got.Op != OpRunCommand {
		t.Fatalf("Op = %q, want %q", got.Op, OpRunCommand)
	}
	if got.Command != "echo" {
		t.Fatalf("Command = %q, want echo", got.Command)
	}
	if len(got.Args) != 2 || got.Args[0] != "a" || got.Args[1] != "b" {
		t.Fatalf("Args = %#v", got.Args)
	}
	if string(got.Stdin) != "hello" {
		t.Fatalf("Stdin = %q, want hello", string(got.Stdin))
	}
}

func TestRunBuildsConfigRequests(t *testing.T) {
	runner := &fakeRunner{}
	cmds := New(runner)

	if err := cmds.Run(context.Background(), []string{"config", "list"}); err != nil {
		t.Fatalf("config list error = %v", err)
	}
	if err := cmds.Run(context.Background(), []string{"config", "set", "chat.enabled", "true"}); err != nil {
		t.Fatalf("config set error = %v", err)
	}

	if len(runner.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(runner.requests))
	}
	if runner.requests[0].Op != OpConfigList {
		t.Fatalf("first op = %q, want %q", runner.requests[0].Op, OpConfigList)
	}
	if runner.requests[1].Op != OpConfigSet {
		t.Fatalf("second op = %q, want %q", runner.requests[1].Op, OpConfigSet)
	}
	if runner.requests[1].Setting != "chat.enabled" || runner.requests[1].Value != "true" {
		t.Fatalf("config set payload = %#v", runner.requests[1])
	}
}

func TestRunBuildsSendStdinRequest(t *testing.T) {
	runner := &fakeRunner{}
	cmds := New(runner)

	withStdin(t, "hello", func() {
		if err := cmds.Run(context.Background(), []string{"sendstdin", "--fenced", "--language", "go"}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})

	got := runner.requests[0]
	if got.Op != OpSendText {
		t.Fatalf("Op = %q, want %q", got.Op, OpSendText)
	}
	if got.Text != "hello" {
		t.Fatalf("Text = %q, want hello", got.Text)
	}
	if !got.Fenced || got.Language != "go" {
		t.Fatalf("sendstdin payload = %#v", got)
	}
}

func TestRunBuildsSendFileRequest(t *testing.T) {
	runner := &fakeRunner{}
	cmds := New(runner)
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := cmds.Run(context.Background(), []string{"sendfile", path, "--caption", "hi", "--type", "text/plain"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := runner.requests[0]
	if got.Op != OpSendFile {
		t.Fatalf("Op = %q, want %q", got.Op, OpSendFile)
	}
	if got.Filename != "note.txt" || got.Caption != "hi" || got.ContentType != "text/plain" || string(got.Content) != "abc" {
		t.Fatalf("sendfile payload = %#v", got)
	}
}
