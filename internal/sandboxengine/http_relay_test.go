package sandboxengine

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRelayTargetURLPreservesPathAndQuery(t *testing.T) {
	t.Parallel()

	got := relayTargetURL(1455, "/callback?code=abc&state=xyz")
	want := "http://127.0.0.1:1455/callback?code=abc&state=xyz"
	if got != want {
		t.Fatalf("relayTargetURL() = %q, want %q", got, want)
	}
}

func TestHTTPRelayCopiesContainerResponse(t *testing.T) {
	t.Parallel()

	var forwardedURL string
	relay, err := startHTTPRelay(context.Background(), httpRelayConfig{
		Addr:          "127.0.0.1:0",
		ContainerName: "ctgbot-test",
		TargetPort:    1455,
		Forward: func(ctx context.Context, targetURL string) ([]byte, error) {
			forwardedURL = targetURL
			return []byte("HTTP/1.1 201 Created\r\nX-Test: ok\r\nContent-Type: text/plain\r\n\r\nrelay body"), nil
		},
	})
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer func() { _ = relay.Close(context.Background()) }()

	resp, err := http.Get("http://" + relay.Addr() + "/callback?code=abc")
	if err != nil {
		t.Fatalf("get relay: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if forwardedURL != "http://127.0.0.1:1455/callback?code=abc" {
		t.Fatalf("forwarded URL = %q", forwardedURL)
	}
	if resp.StatusCode != http.StatusCreated || resp.Header.Get("X-Test") != "ok" || string(body) != "relay body" {
		t.Fatalf("unexpected response status=%d header=%q body=%q", resp.StatusCode, resp.Header.Get("X-Test"), string(body))
	}
}

func TestHTTPRelayRejectsNonGET(t *testing.T) {
	t.Parallel()

	relay, err := startHTTPRelay(context.Background(), httpRelayConfig{
		Addr:          "127.0.0.1:0",
		ContainerName: "ctgbot-test",
		TargetPort:    1455,
		Forward: func(ctx context.Context, targetURL string) ([]byte, error) {
			return []byte("HTTP/1.1 200 OK\r\n\r\n"), nil
		},
	})
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer func() { _ = relay.Close(context.Background()) }()

	resp, err := http.Post("http://"+relay.Addr()+"/callback", "text/plain", strings.NewReader("body"))
	if err != nil {
		t.Fatalf("post relay: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestHTTPRelayReturnsBadGatewayForInvalidContainerResponse(t *testing.T) {
	t.Parallel()

	relay, err := startHTTPRelay(context.Background(), httpRelayConfig{
		Addr:          "127.0.0.1:0",
		ContainerName: "ctgbot-test",
		TargetPort:    1455,
		Forward: func(ctx context.Context, targetURL string) ([]byte, error) {
			return []byte("not http"), nil
		},
	})
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer func() { _ = relay.Close(context.Background()) }()

	resp, err := http.Get("http://" + relay.Addr() + "/callback")
	if err != nil {
		t.Fatalf("get relay: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadGateway)
	}
}

func TestHTTPRelayTimeoutClosesServer(t *testing.T) {
	relay, err := startHTTPRelay(context.Background(), httpRelayConfig{
		Addr:          "127.0.0.1:0",
		ContainerName: "ctgbot-test",
		TargetPort:    1455,
		Timeout:       10 * time.Millisecond,
		Forward: func(ctx context.Context, targetURL string) ([]byte, error) {
			return []byte("HTTP/1.1 200 OK\r\n\r\n"), nil
		},
	})
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	_, err = http.Get("http://" + relay.Addr() + "/callback")
	if err == nil {
		t.Fatal("expected closed relay request to fail")
	}
}
