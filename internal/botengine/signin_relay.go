package botengine

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"time"
)

type signinRelay struct {
	server        *http.Server
	containerName string
	port          int
	logger        *log.Logger
}

func startSigninRelay(containerName string, port int, logger *log.Logger) (*signinRelay, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}

	relay := &signinRelay{
		containerName: containerName,
		port:          port,
		logger:        logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", relay.handle)

	relay.server = &http.Server{Handler: mux}
	go func() {
		if err := relay.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			relay.logf("signin relay serve error: %v", err)
		}
	}()

	relay.logf("signin relay listening on %s for container=%s", addr, containerName)
	return relay, nil
}

func (r *signinRelay) Close(ctx context.Context) error {
	if r == nil || r.server == nil {
		return nil
	}
	return r.server.Shutdown(ctx)
}

func (r *signinRelay) handle(w http.ResponseWriter, req *http.Request) {
	raw, err := r.forward(req.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(raw)), req)
	if err != nil {
		http.Error(w, "invalid response from container login server: "+err.Error(), http.StatusBadGateway)
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

func (r *signinRelay) forward(ctx context.Context, req *http.Request) ([]byte, error) {
	target := (&url.URL{
		Scheme:   "http",
		Host:     fmt.Sprintf("127.0.0.1:%d", r.port),
		Path:     req.URL.Path,
		RawQuery: req.URL.RawQuery,
	}).String()

	var lastErr error
	for range 20 {
		cmd := exec.CommandContext(ctx, "docker", "exec", r.containerName, "curl", "-isS", target)
		out, err := cmd.CombinedOutput()
		if err == nil {
			return out, nil
		}
		lastErr = fmt.Errorf("%w: %s", err, string(bytes.TrimSpace(out)))
		time.Sleep(200 * time.Millisecond)
	}
	return nil, fmt.Errorf("forward callback to container: %w", lastErr)
}

func (r *signinRelay) logf(format string, args ...any) {
	if r.logger != nil {
		r.logger.Printf(format, args...)
	}
}
