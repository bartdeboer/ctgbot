package gobtransport

import (
	"bytes"
	"context"
	"encoding/gob"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/hostbridge/transport"
)

func TestServerRejectsCommandRequestOverTransportLimit(t *testing.T) {
	var payload bytes.Buffer
	if err := gob.NewEncoder(&payload).Encode(hostbridge.CommandRequest{
		Request: commandengine.Request{Stdin: strings.Repeat("x", 1024)},
	}); err != nil {
		t.Fatalf("encode request: %v", err)
	}
	handler := &countingHandler{}
	conn := &memoryConn{reader: bytes.NewReader(payload.Bytes())}
	err := (&Server{Handler: handler, MaxRequestBytes: 64}).ServeConn(context.Background(), conn)
	if err == nil || !strings.Contains(err.Error(), "request too large") {
		t.Fatalf("ServeConn() error = %v, want request limit", err)
	}
	if handler.calls != 0 {
		t.Fatalf("handler calls = %d, want zero", handler.calls)
	}
}

type countingHandler struct{ calls int }

func (h *countingHandler) HandleCommand(context.Context, transport.PeerIdentity, hostbridge.CommandRequest) hostbridge.CommandResponse {
	h.calls++
	return hostbridge.CommandResponse{}
}

type memoryConn struct {
	reader *bytes.Reader
	writer bytes.Buffer
}

func (c *memoryConn) Read(p []byte) (int, error)  { return c.reader.Read(p) }
func (c *memoryConn) Write(p []byte) (int, error) { return c.writer.Write(p) }
func (c *memoryConn) Close() error                { return nil }
