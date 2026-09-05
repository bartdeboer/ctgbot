package backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

// nativeRuntime owns exactly one foreground service, including its Wait call.
// All lifecycle transitions are serialized; the child outlives request contexts.
type nativeRuntime struct {
	authority *atomic.Bool
	shutdown  context.Context
	mu        sync.Mutex
	profile   runtimepkg.Profile
	spec      ServiceSpec
	env       []string
	closed    bool
	cmd       *exec.Cmd
	done      chan struct{}
	exitErr   error // written before done closes
	logs      tailBuffer
}

func (r *nativeRuntime) ComponentProfile() runtimepkg.Profile { return r.profile }
func (r *nativeRuntime) BaseURL() string                      { return r.spec.BaseURL }
func (r *nativeRuntime) alive() bool {
	if r.cmd == nil {
		return false
	}
	select {
	case <-r.done:
		return false
	default:
		return true
	}
}
func (r *nativeRuntime) statusLocked() runtimepkg.Status {
	state := "stopped"
	if r.alive() {
		state = "running"
	}
	return runtimepkg.Status{Name: r.spec.Identity, State: state, RuntimeProfilePath: r.profile.Path}
}
func (r *nativeRuntime) Status(ctx context.Context) (runtimepkg.Status, error) {
	if err := r.requireResident(); err != nil {
		return runtimepkg.Status{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.statusLocked(), nil
}

func (r *nativeRuntime) Start(ctx context.Context) (runtimepkg.Status, error) {
	if err := r.requireResident(); err != nil {
		return runtimepkg.Status{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.shutdown.Err() != nil {
		return runtimepkg.Status{}, fmt.Errorf("native backend is closed")
	}
	if err := ctx.Err(); err != nil {
		return runtimepkg.Status{}, err
	}
	if r.alive() {
		return r.statusLocked(), nil
	}
	if r.cmd != nil {
		if err := r.finishStop(nil); err != nil {
			return runtimepkg.Status{}, err
		}
	}
	// Refuse occupied ports; never adopt or kill an unrelated listener.
	u, err := url.Parse(r.spec.BaseURL)
	if err != nil {
		return runtimepkg.Status{}, err
	}
	if u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" {
		return runtimepkg.Status{}, fmt.Errorf("native backend requires an explicit loopback HTTP port")
	}
	listener, err := net.Listen("tcp", u.Host)
	if err != nil {
		return runtimepkg.Status{}, fmt.Errorf("native backend port unavailable (check for an orphan or another service): %w", err)
	}
	listener.Close()
	args := append(append([]string(nil), r.spec.Native.Args...), r.spec.Cmd...)
	cmd := exec.Command(r.spec.Native.Executable, args...)
	if err := prepareNativeProcess(cmd); err != nil {
		return runtimepkg.Status{}, err
	}
	cmd.Dir = r.profile.Path
	cmd.Env = runtimepkg.MergeEnv(os.Environ(), runtimepkg.MergeEnv(r.spec.Env, r.env))
	r.logs.reset()
	cmd.Stdout, cmd.Stderr = &r.logs, &r.logs
	// Bound Wait if a descendant inherits output descriptors after the child exits.
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return runtimepkg.Status{}, fmt.Errorf("start native backend: %w", err)
	}
	r.cmd, r.done = cmd, make(chan struct{})
	done := r.done
	go func() { r.exitErr = cmd.Wait(); close(done) }()
	if err := r.waitNativeReady(ctx); err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		stopErr := r.stopLocked(cleanup)
		return runtimepkg.Status{}, fmt.Errorf("native backend readiness: %w (cleanup: %v)\n%s", err, stopErr, r.logs.String())
	}
	return r.statusLocked(), nil
}

func (r *nativeRuntime) waitNativeReady(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	stopShutdown := context.AfterFunc(r.shutdown, cancel)
	defer stopShutdown()
	client := &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-r.done:
			return fmt.Errorf("server exited: %v", r.exitErr)
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.spec.HealthURL, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				// Allow a child losing a bind race to report its failure before success.
				select {
				case <-r.done:
					return fmt.Errorf("server exited: %v", r.exitErr)
				case <-ctx.Done():
					return ctx.Err()
				case <-ticker.C:
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.done:
			return fmt.Errorf("server exited: %v", r.exitErr)
		case <-ticker.C:
		}
	}
}

func (r *nativeRuntime) Stop(ctx context.Context) error {
	if err := r.requireResident(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopLocked(ctx)
}
func (r *nativeRuntime) Refresh(ctx context.Context) error { return r.Stop(ctx) }

// Successful stop certifies only the direct child's completion. Descendants are
// the executable/wrapper's responsibility, not a process-tree promise.
func (r *nativeRuntime) stopLocked(ctx context.Context) error {
	if r.cmd == nil {
		return nil
	}
	if !r.alive() {
		return r.finishStop(nil)
	}
	termErr := signalNativeProcess(r.cmd, false)
	if errors.Is(termErr, os.ErrProcessDone) {
		termErr = nil
	}
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-r.done:
		return r.finishStop(termErr)
	case <-timer.C:
	case <-ctx.Done():
	}
	killErr := signalNativeProcess(r.cmd, true)
	if errors.Is(killErr, os.ErrProcessDone) {
		killErr = nil
	}
	signalErr := errors.Join(termErr, killErr)
	reap := time.NewTimer(2 * time.Second)
	defer reap.Stop()
	select {
	case <-r.done:
		return r.finishStop(signalErr)
	case <-reap.C:
		return errors.Join(signalErr, fmt.Errorf("native direct child did not complete Wait after kill"))
	}
}

// Only called after done closes, synchronizing access to Wait's result.
func (r *nativeRuntime) finishStop(signalErr error) error {
	waitErr := r.exitErr
	var exitErr *exec.ExitError
	// A non-zero child exit is expected on termination. Pipe/wait failures are not.
	if errors.As(waitErr, &exitErr) {
		waitErr = nil
	}
	err := errors.Join(signalErr, waitErr)
	if err == nil {
		r.cmd = nil
	}
	return err
}

// Keep only the latest 64 KiB, avoiding unlimited child-output growth.
type tailBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if len(p) > 65536 {
		p = p[len(p)-65536:]
	}
	b.data = append(b.data, p...)
	if len(b.data) > 65536 {
		b.data = append([]byte(nil), b.data[len(b.data)-65536:]...)
	}
	return n, nil
}
func (b *tailBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.data) }
func (b *tailBuffer) reset()         { b.mu.Lock(); defer b.mu.Unlock(); b.data = nil }

func (r *nativeRuntime) requireResident() error {
	if !r.authority.Load() {
		return fmt.Errorf("native inference and lifecycle require resident ctgbot run; send this command through its chat or Hostbridge, not the standalone CLI")
	}
	return nil
}
