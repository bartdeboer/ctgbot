package chatcommands

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type fakeRunner struct {
	commands []Command
}

func (f *fakeRunner) Execute(cmd Command) error {
	f.commands = append(f.commands, cmd)
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

func TestRunBuildsRunCommand(t *testing.T) {
	runner := &fakeRunner{}
	cmds := New(runner)

	withStdin(t, "hello", func() {
		if err := cmds.Run(context.Background(), []string{"run", "echo", "a", "b"}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})

	if len(runner.commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(runner.commands))
	}
	cmd, ok := runner.commands[0].(RunCommand)
	if !ok {
		t.Fatalf("command type = %T, want RunCommand", runner.commands[0])
	}
	if cmd.Command != "echo" || len(cmd.Args) != 2 || cmd.Args[0] != "a" || cmd.Args[1] != "b" {
		t.Fatalf("run command = %#v", cmd)
	}
	if string(cmd.Stdin) != "hello" {
		t.Fatalf("Stdin = %q, want hello", string(cmd.Stdin))
	}
}

func TestRunBuildsConfigCommands(t *testing.T) {
	runner := &fakeRunner{}
	cmds := New(runner)

	if err := cmds.Run(context.Background(), []string{"config", "list"}); err != nil {
		t.Fatalf("config list error = %v", err)
	}
	if err := cmds.Run(context.Background(), []string{"config", "set", "chat.enabled", "true"}); err != nil {
		t.Fatalf("config set error = %v", err)
	}

	if _, ok := runner.commands[0].(ConfigList); !ok {
		t.Fatalf("first command type = %T, want ConfigList", runner.commands[0])
	}
	cfg, ok := runner.commands[1].(ConfigSet)
	if !ok {
		t.Fatalf("second command type = %T, want ConfigSet", runner.commands[1])
	}
	if cfg.Setting != "chat.enabled" || cfg.Value != "true" {
		t.Fatalf("config set = %#v", cfg)
	}
}

func TestRunBuildsSendText(t *testing.T) {
	runner := &fakeRunner{}
	cmds := New(runner)

	withStdin(t, "hello", func() {
		if err := cmds.Run(context.Background(), []string{"sendstdin", "--fenced", "--language", "go"}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})

	cmd, ok := runner.commands[0].(SendText)
	if !ok {
		t.Fatalf("command type = %T, want SendText", runner.commands[0])
	}
	if cmd.Text != "hello" || !cmd.Fenced || cmd.Language != "go" {
		t.Fatalf("send text = %#v", cmd)
	}
}

func TestRunBuildsSendFile(t *testing.T) {
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

	cmd, ok := runner.commands[0].(SendFile)
	if !ok {
		t.Fatalf("command type = %T, want SendFile", runner.commands[0])
	}
	if cmd.Filename != "note.txt" || cmd.Caption != "hi" || cmd.ContentType != "text/plain" || string(cmd.Content) != "abc" {
		t.Fatalf("send file = %#v", cmd)
	}
}
