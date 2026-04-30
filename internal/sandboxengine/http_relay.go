package sandboxengine

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bartdeboer/ctgbot/internal/containerengine"
)

type HTTPRelay interface {
	Addr() string
	Close(ctx context.Context) error
}

type httpRelay struct {
	addr   string
	server *http.Server
	logger *log.Logger
	once   sync.Once
}

type httpRelayConfig struct {
	Addr          string
	ContainerName string
	TargetPort    int
	Timeout       time.Duration
	Logger        *log.Logger
	Forward       httpRelayForwarder
}

type httpRelayForwarder func(ctx context.Context, targetURL string) ([]byte, error)

func (s *Sandbox) OpenHTTPRelayPort(ctx context.Context, port int, timeout time.Duration) (HTTPRelay, error) {
	if s == nil {
		return nil, fmt.Errorf("missing sandbox")
	}
	if strings.TrimSpace(s.Name) == "" {
		return nil, fmt.Errorf("missing sandbox name")
	}
	if port <= 0 {
		return nil, fmt.Errorf("invalid relay port: %d", port)
	}
	container := s.ensureContainer()
	if container == nil {
		return nil, fmt.Errorf("missing backing container")
	}

	var logger *log.Logger
	if s.docker != nil {
		logger = s.docker.Logger
	}
	return startHTTPRelay(ctx, httpRelayConfig{
		Addr:          fmt.Sprintf("127.0.0.1:%d", port),
		ContainerName: s.Name,
		TargetPort:    port,
		Timeout:       timeout,
		Logger:        logger,
		Forward:       dockerHTTPRelayForwarder(container),
	})
}

func startHTTPRelay(ctx context.Context, cfg httpRelayConfig) (*httpRelay, error) {
	addr := strings.TrimSpace(cfg.Addr)
	if addr == "" {
		return nil, fmt.Errorf("missing relay address")
	}
	if strings.TrimSpace(cfg.ContainerName) == "" {
		return nil, fmt.Errorf("missing relay container name")
	}
	if cfg.TargetPort <= 0 {
		return nil, fmt.Errorf("invalid target port: %d", cfg.TargetPort)
	}
	if cfg.Forward == nil {
		return nil, fmt.Errorf("missing relay forwarder")
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}

	relay := &httpRelay{addr: ln.Addr().String(), logger: cfg.Logger}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		relay.handle(w, req, cfg)
	})
	relay.server = &http.Server{Handler: mux}

	go func() {
		if err := relay.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			relay.logf("http relay serve error addr=%s err=%v", relay.addr, err)
		}
	}()

	if cfg.Timeout > 0 || ctx != nil {
		go relay.closeWhenDone(ctx, cfg.Timeout)
	}
	relay.logf("http relay listening addr=%s container=%s target_port=%d", relay.addr, cfg.ContainerName, cfg.TargetPort)
	return relay, nil
}

func (r *httpRelay) Addr() string {
	if r == nil {
		return ""
	}
	return r.addr
}

func (r *httpRelay) Close(ctx context.Context) error {
	if r == nil || r.server == nil {
		return nil
	}
	var err error
	r.once.Do(func() {
		if ctx == nil {
			ctx = context.Background()
		}
		err = r.server.Shutdown(ctx)
	})
	return err
}

func (r *httpRelay) closeWhenDone(ctx context.Context, timeout time.Duration) {
	var timer <-chan time.Time
	if timeout > 0 {
		timer = time.After(timeout)
	}
	if ctx == nil {
		<-timer
		_ = r.Close(context.Background())
		return
	}
	select {
	case <-ctx.Done():
	case <-timer:
	}
	_ = r.Close(context.Background())
}

func (r *httpRelay) handle(w http.ResponseWriter, req *http.Request, cfg httpRelayConfig) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	targetURL := relayTargetURL(cfg.TargetPort, req.URL.RequestURI())
	raw, err := cfg.Forward(req.Context(), targetURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	copyRawHTTPResponse(w, req, raw)
}

func relayTargetURL(port int, requestURI string) string {
	if requestURI == "" {
		requestURI = "/"
	}
	if !strings.HasPrefix(requestURI, "/") {
		requestURI = "/" + requestURI
	}
	return fmt.Sprintf("http://127.0.0.1:%d%s", port, requestURI)
}

func copyRawHTTPResponse(w http.ResponseWriter, req *http.Request, raw []byte) {
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(raw)), req)
	if err != nil {
		http.Error(w, "invalid response from container callback server: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func dockerHTTPRelayForwarder(container *containerengine.Container) httpRelayForwarder {
	return func(ctx context.Context, targetURL string) ([]byte, error) {
		var lastErr error
		for range 20 {
			out, err := container.CombinedOutput(ctx, containerengine.ExecOptions{}, "curl", "-isS", targetURL)
			if err == nil {
				return out, nil
			}
			lastErr = fmt.Errorf("%w: %s", err, string(bytes.TrimSpace(out)))
			time.Sleep(200 * time.Millisecond)
		}
		return nil, fmt.Errorf("forward callback to container: %w", lastErr)
	}
}

func (r *httpRelay) logf(format string, args ...any) {
	if r != nil && r.logger != nil {
		r.logger.Printf(format, args...)
	}
}
