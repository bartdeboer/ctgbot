package hostbridgev2_test

import (
	"context"
	"net"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/chatcommands"
	"github.com/bartdeboer/ctgbot/internal/hostbridgev2"
	clientpkg "github.com/bartdeboer/ctgbot/internal/hostbridgev2/client"
	serverpkg "github.com/bartdeboer/ctgbot/internal/hostbridgev2/server"
)

type fakeRunner struct {
	requests []chatcommands.Request
	result   chatcommands.Result
}

func (f *fakeRunner) Execute(_ context.Context, req chatcommands.Request) (chatcommands.Result, error) {
	f.requests = append(f.requests, req)
	return f.result, nil
}

func TestClientServerRoundTrip(t *testing.T) {
	runner := &fakeRunner{result: chatcommands.Result{Text: "ok"}}
	server := serverpkg.New(runner)
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	go func() {
		_ = server.ServeConn(context.Background(), serverConn)
	}()

	resp, err := clientpkg.DoConn(clientConn, hostbridgev2.Request{Request: chatcommands.Request{Command: chatcommands.ConfigList{}}})
	if err != nil {
		t.Fatalf("DoConn() error = %v", err)
	}
	if resp.Result.Text != "ok" {
		t.Fatalf("resp.Result.Text = %q, want ok", resp.Result.Text)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(runner.requests))
	}
	if _, ok := runner.requests[0].Command.(chatcommands.ConfigList); !ok {
		t.Fatalf("command type = %T, want ConfigList", runner.requests[0].Command)
	}
}
