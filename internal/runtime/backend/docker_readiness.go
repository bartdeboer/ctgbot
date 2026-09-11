package backend

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/containerengine"
)

// Docker readiness checks the container as well as HTTP: a stale listener must
// not hide an exited backend. These bounds deliberately do not change native probing.
func (r *Runtime) waitReady(ctx context.Context) error {
	healthURL := strings.TrimSpace(r.service.HealthURL)
	if healthURL == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var lastErr error
	for {
		state, err := r.readinessState(ctx)
		if err != nil {
			return err
		}
		if state == containerengine.StateRunning {
			lastErr = probeDockerHealth(ctx, healthURL)
			if lastErr == nil {
				state, err = r.readinessState(ctx)
				if err != nil {
					return err
				}
				if state == containerengine.StateRunning {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("backend %s not ready (last health error: %v): %w", r.containerName(), lastErr, ctx.Err())
		case <-time.After(time.Second):
		}
	}
}

func (r *Runtime) readinessState(ctx context.Context) (containerengine.State, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	state, err := r.container().InspectState(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return state, fmt.Errorf("inspect backend %s: %w", r.containerName(), ctx.Err())
		}
		return state, err
	}
	switch state {
	case containerengine.StateMissing, containerengine.StateExited, containerengine.State("dead"):
		label := string(state)
		if state == containerengine.StateMissing {
			label = "missing"
		}
		return state, fmt.Errorf("backend %s is %s before readiness; inspect logs with: docker logs --tail 50 %s", r.containerName(), label, r.containerName())
	}
	return state, nil
}

func probeDockerHealth(ctx context.Context, url string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := newHealthRequest(ctx, url)
	if err != nil {
		return err
	}
	client := http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	const maxBody = 64 << 10
	n, err := io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return err
	}
	if n > maxBody {
		return fmt.Errorf("backend health response exceeds %d bytes", maxBody)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health status %s", resp.Status)
	}
	return nil
}
