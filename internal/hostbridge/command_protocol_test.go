package hostbridge_test

import (
	"context"
	"encoding/gob"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func TestCommandProtocolCanParseTransmitDecodeAndExecute(t *testing.T) {
	ctx := context.Background()
	router, err := commandengine.NewRouter(schemacommands.ExampleCommands(), commandengine.SourceHostbridge)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	registry := commandengine.NewRegistry()
	if err := commandengine.Register[schemacommands.Echo](registry, func(_ context.Context, req commandengine.Request, cmd schemacommands.Echo) (commandengine.Result, error) {
		if req.Context.Source != commandengine.SourceHostbridge {
			t.Fatalf("source = %q, want hostbridge", req.Context.Source)
		}
		return commandengine.Result{Text: "remote:" + cmd.Text}, nil
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	parsed, err := router.Parse(ctx, commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{
				ID:    "agent-1",
				Roles: []simplerbac.Role{simplerbac.RoleAgent},
			},
		},
	}, []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if _, ok := parsed.Command.(schemacommands.Echo); !ok {
		t.Fatalf("parsed command type = %T, want commands.Echo", parsed.Command)
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	serverDone := make(chan error, 1)
	go func() {
		defer serverConn.Close()
		var req hostbridge.CommandRequest
		if err := gob.NewDecoder(serverConn).Decode(&req); err != nil {
			serverDone <- err
			return
		}
		if _, ok := req.Request.Command.(schemacommands.Echo); !ok {
			serverDone <- fmt.Errorf("decoded command type = %T, want commands.Echo", req.Request.Command)
			return
		}
		result, err := registry.Execute(ctx, req.Request)
		resp := hostbridge.CommandResponse{Result: result}
		if err != nil {
			resp.Error = err.Error()
		}
		if err := gob.NewEncoder(serverConn).Encode(resp); err != nil {
			serverDone <- err
			return
		}
		serverDone <- nil
	}()

	if err := gob.NewEncoder(clientConn).Encode(hostbridge.CommandRequest{Request: parsed}); err != nil {
		t.Fatalf("encode request: %v", err)
	}
	var resp hostbridge.CommandResponse
	if err := gob.NewDecoder(clientConn).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("server error = %v", err)
	}
	if strings.TrimSpace(resp.Error) != "" {
		t.Fatalf("response error = %q", resp.Error)
	}
	if resp.Result.Text != "remote:hello" {
		t.Fatalf("result text = %q, want remote:hello", resp.Result.Text)
	}
}
