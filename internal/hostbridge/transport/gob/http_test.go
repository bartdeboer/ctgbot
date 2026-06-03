package gobtransport

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/hostbridge/transport"
	httptransport "github.com/bartdeboer/ctgbot/internal/hostbridge/transport/http"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

func TestTypedCommandRunsOverGobHTTPTransport(t *testing.T) {
	server := httptest.NewServer(&HTTPHandler{Handler: echoCommandHandler{}})
	defer server.Close()

	runner := &CommandRunner{Transport: &httptransport.ByteTransport{URL: server.URL, Client: server.Client()}}
	resp, err := runner.RunCommand(context.Background(), hostbridge.CommandRequest{
		Request: commandengine.Request{Command: schemacommands.Echo{Text: "typed"}},
	})
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if got, want := resp.Result.Text, "typed"; got != want {
		t.Fatalf("result = %q, want %q", got, want)
	}
}

type echoCommandHandler struct{}

func (echoCommandHandler) HandleCommand(ctx context.Context, peer transport.PeerIdentity, req hostbridge.CommandRequest) hostbridge.CommandResponse {
	_, _ = ctx, peer
	cmd, ok := req.Request.Command.(schemacommands.Echo)
	if !ok {
		return hostbridge.CommandResponse{Error: "wrong command type"}
	}
	return hostbridge.CommandResponse{Result: commandengine.Result{Text: cmd.Text}}
}
