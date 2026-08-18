package hostbridge_test

import (
	"bytes"
	"context"
	"encoding/gob"
	"net"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	serverpkg "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	gobtransport "github.com/bartdeboer/ctgbot/internal/hostbridge/transport/gob"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

type staticDialer struct {
	conn net.Conn
}

func (d staticDialer) Dial(context.Context, string) (net.Conn, error) {
	return d.conn, nil
}

func TestCommandClientServerRoundTripExecutesConcreteCommand(t *testing.T) {
	registry := commandengine.NewRegistry()
	if err := commandengine.Register[schemacommands.RunCommand](registry, func(_ context.Context, req commandengine.Request, cmd schemacommands.RunCommand) (commandengine.Result, error) {
		return commandengine.Result{Text: cmd.Command + " " + joinArgs(cmd.Args)}, nil
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	server := serverpkg.NewCommandServer(commandengine.NewEngine(nil, registry))
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- (&gobtransport.Server{Handler: server}).ServeConn(context.Background(), serverConn)
	}()

	runner := &gobtransport.CommandRunner{
		Transport: &gobtransport.DialTransport{Dialer: staticDialer{conn: clientConn}},
	}
	resp, err := runner.RunCommand(context.Background(), hostbridge.CommandRequest{
		Request: commandengine.Request{
			Command: schemacommands.RunCommand{
				Command: "echo",
				Args:    []string{"hello"},
			},
		},
	})
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("server error = %v", err)
	}
	if resp.Result.Text != "echo hello" {
		t.Fatalf("response text = %q, want echo hello", resp.Result.Text)
	}
}

func TestCommandClientServerRoundTripPreservesExecutionAndLegacyError(t *testing.T) {
	registry := commandengine.NewRegistry()
	if err := commandengine.Register[schemacommands.RunCommand](registry, func(context.Context, commandengine.Request, schemacommands.RunCommand) (commandengine.Result, error) {
		return commandengine.Result{Execution: &commandengine.ExecutionResult{
			Stdout:   "{\"status\":\"degraded\"}\n",
			Stderr:   "health diagnostic\n",
			ExitCode: 2,
		}}, nil
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	server := serverpkg.NewCommandServer(commandengine.NewEngine(nil, registry))
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- (&gobtransport.Server{Handler: server}).ServeConn(context.Background(), serverConn)
	}()

	runner := &gobtransport.CommandRunner{
		Transport: &gobtransport.DialTransport{Dialer: staticDialer{conn: clientConn}},
	}
	resp, err := runner.RunCommand(context.Background(), hostbridge.CommandRequest{
		Request: commandengine.Request{Command: schemacommands.RunCommand{Command: "probe"}},
	})
	if err == nil || err.Error() != "exit status 2: health diagnostic" {
		t.Fatalf("RunCommand() error = %v, want compatibility error", err)
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("server error = %v", err)
	}
	if resp.Result.Execution == nil || resp.Result.Execution.ExitCode != 2 {
		t.Fatalf("execution = %+v, want exit 2", resp.Result.Execution)
	}
	if got, want := resp.Error, "exit status 2: health diagnostic"; got != want {
		t.Fatalf("legacy error = %q, want %q", got, want)
	}

	var wire bytes.Buffer
	if err := gob.NewEncoder(&wire).Encode(resp); err != nil {
		t.Fatalf("encode current response: %v", err)
	}
	var legacy legacyCommandResponse
	if err := gob.NewDecoder(&wire).Decode(&legacy); err != nil {
		t.Fatalf("decode legacy response: %v", err)
	}
	if got, want := legacy.Error, "exit status 2: health diagnostic"; got != want {
		t.Fatalf("legacy decoded error = %q, want %q", got, want)
	}
}

type legacyCommandResponse struct {
	Result legacyCommandResult
	Error  string
}

type legacyCommandResult struct {
	Text              string
	PassthroughPrompt string
}

func joinArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	out := args[0]
	for _, arg := range args[1:] {
		out += " " + arg
	}
	return out
}
