package gobtransport

import (
	"bytes"
	"context"
	"encoding/gob"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
	"net/http/httptest"
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

func TestDefaultRequestBudgetAcceptsMediaAboveOldLimit(t *testing.T) {
	if DefaultMaxCommandRequestBytes != 50<<20 {
		t.Fatal("default envelope budget must be 50 MiB")
	}
	var payload bytes.Buffer
	if err := gob.NewEncoder(&payload).Encode(hostbridge.CommandRequest{
		Request: commandengine.Request{Command: schemacommands.SendMedia{
			Filename: "fixture.mp4", ContentType: "video/mp4", Content: make([]byte, 17<<20),
		}},
	}); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"gob", "http"} {
		t.Run(mode, func(t *testing.T) {
			handler := &mediaSizeHandler{t: t, want: 17 << 20}
			if mode == "gob" {
				conn := &memoryConn{reader: bytes.NewReader(payload.Bytes())}
				if err := (&Server{Handler: handler}).ServeConn(context.Background(), conn); err != nil {
					t.Fatal(err)
				}
			} else {
				rec := httptest.NewRecorder()
				(&HTTPHandler{Handler: handler}).ServeHTTP(rec, httptest.NewRequest("POST", "/", bytes.NewReader(payload.Bytes())))
				if rec.Code != 200 {
					t.Fatalf("status=%d", rec.Code)
				}
			}
			if handler.calls != 1 {
				t.Fatalf("handler calls=%d", handler.calls)
			}
		})
	}
}

func TestEncodedRequestBoundary(t *testing.T) {
	var payload bytes.Buffer
	if err := gob.NewEncoder(&payload).Encode(hostbridge.CommandRequest{Request: commandengine.Request{Stdin: "small"}}); err != nil {
		t.Fatal(err)
	}
	// Same enforcement with explicit small limits avoids a 50 MiB rejection fixture.
	for _, over := range []bool{false, true} {
		limit := int64(payload.Len())
		if over {
			limit--
		}
		handler := &countingHandler{}
		conn := &memoryConn{reader: bytes.NewReader(payload.Bytes())}
		err := (&Server{Handler: handler, MaxRequestBytes: limit}).ServeConn(context.Background(), conn)
		if (err != nil) != over {
			t.Fatalf("over=%v err=%v", over, err)
		}
		want := 1
		if over {
			want = 0
		}
		if handler.calls != want {
			t.Fatalf("over=%v calls=%d", over, handler.calls)
		}
		handler = &countingHandler{}
		rec := httptest.NewRecorder()
		(&HTTPHandler{Handler: handler, MaxBodySize: limit}).ServeHTTP(rec, httptest.NewRequest("POST", "/", bytes.NewReader(payload.Bytes())))
		status := 200
		if over {
			status = 413
		}
		if rec.Code != status || handler.calls != want {
			t.Fatalf("over=%v status=%d calls=%d", over, rec.Code, handler.calls)
		}
	}
}

type mediaSizeHandler struct {
	t     *testing.T
	want  int
	calls int
}

func (h *mediaSizeHandler) HandleCommand(_ context.Context, _ transport.PeerIdentity, req hostbridge.CommandRequest) hostbridge.CommandResponse {
	h.calls++
	media, ok := req.Request.Command.(schemacommands.SendMedia)
	if !ok || len(media.Content) != h.want {
		h.t.Fatal("media payload not preserved")
	}
	return hostbridge.CommandResponse{}
}
