package commands

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func TestHostbridgeRunDoesNotReadStdin(t *testing.T) {
	originalStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}
	t.Cleanup(func() {
		os.Stdin = originalStdin
		_ = reader.Close()
		_ = writer.Close()
	})
	os.Stdin = reader

	router, err := commandengine.NewRouter(HostbridgeCommands(), commandengine.SourceHostbridge)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	done := make(chan commandengine.Request, 1)
	errs := make(chan error, 1)
	go func() {
		req, err := router.Parse(context.Background(), commandengine.Request{
			Context: commandengine.Context{
				Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
			},
		}, []string{"run", "echo", "hello"})
		if err != nil {
			errs <- err
			return
		}
		done <- req
	}()

	select {
	case err := <-errs:
		t.Fatalf("Parse() error = %v", err)
	case req := <-done:
		cmd, ok := req.Command.(RunCommand)
		if !ok {
			t.Fatalf("command = %T, want RunCommand", req.Command)
		}
		if cmd.Command != "echo" || !reflect.DeepEqual(cmd.Args, []string{"hello"}) || len(cmd.Stdin) != 0 {
			t.Fatalf("command = %#v, want run echo without stdin", cmd)
		}
	case <-time.After(200 * time.Millisecond):
		_ = writer.Close()
		t.Fatal("Parse() blocked reading stdin")
	}
}

func TestHostbridgeSendFileParsesMediaCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	router, err := commandengine.NewRouter(HostbridgeCommands(), commandengine.SourceHostbridge)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}

	req, err := router.Parse(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
		},
	}, []string{"sendfile", path, "-caption", "a note", "-syntax", "markdown"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	cmd, ok := req.Command.(SendMedia)
	if !ok {
		t.Fatalf("command = %T, want SendMedia", req.Command)
	}
	if cmd.Filename != "note.txt" || cmd.Caption != "a note" || cmd.ContentType != "text/plain" || cmd.Syntax != "markdown" || string(cmd.Content) != "hello" {
		t.Fatalf("command = %#v, want parsed send media", cmd)
	}
}

func TestHostbridgeSendStdinParsesMediaCommand(t *testing.T) {
	originalStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}
	t.Cleanup(func() {
		os.Stdin = originalStdin
		_ = reader.Close()
	})
	os.Stdin = reader
	go func() {
		_, _ = io.WriteString(writer, "hello from stdin")
		_ = writer.Close()
	}()

	router, err := commandengine.NewRouter(HostbridgeCommands(), commandengine.SourceHostbridge)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	req, err := router.Parse(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
		},
	}, []string{"sendstdin", "-caption", "a note", "-syntax", "markdown"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	cmd, ok := req.Command.(SendMedia)
	if !ok {
		t.Fatalf("command = %T, want SendMedia", req.Command)
	}
	if cmd.Filename != "stdin.txt" || cmd.Caption != "a note" || cmd.ContentType != "text/plain" || cmd.Syntax != "markdown" || string(cmd.Content) != "hello from stdin" {
		t.Fatalf("command = %#v, want parsed stdin media", cmd)
	}
}

func TestHostbridgeCommandsRequireAgentRole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	router, err := commandengine.NewRouter(HostbridgeCommands(), commandengine.SourceHostbridge)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	_, err = router.Parse(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleUser}},
		},
	}, []string{"sendfile", path})
	if err == nil {
		t.Fatal("Parse() as user succeeded, want RBAC denial")
	}
}
