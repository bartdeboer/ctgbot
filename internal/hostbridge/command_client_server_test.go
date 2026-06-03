package hostbridge_test

import (
	"context"
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
		Transport: &gobtransport.ConnTransport{Dialer: staticDialer{conn: clientConn}},
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
