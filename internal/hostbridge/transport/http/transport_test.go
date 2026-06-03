package httptransport

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestByteTransportPostsPayloadAndReturnsResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", req.Method)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		if got, want := string(body), "request"; got != want {
			t.Fatalf("body = %q, want %q", got, want)
		}
		_, _ = w.Write([]byte("response"))
	}))
	defer server.Close()

	transport := &ByteTransport{URL: server.URL, Client: server.Client()}
	payload, err := transport.Send(context.Background(), []byte("request"))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if got, want := string(payload), "response"; got != want {
		t.Fatalf("response = %q, want %q", got, want)
	}
}
