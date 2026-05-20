package sandboxengine

import (
	"context"
	"testing"
	"time"
)

func TestSandboxSessionTracksConcurrentLeases(t *testing.T) {
	t.Parallel()
	sessions := &sandboxSessions{states: map[string]*sandboxSessionState{}}
	name := "ctgbot-session-test"

	sessions.begin(name)
	sessions.begin(name)
	if got := sessions.states[name].active; got != 2 {
		t.Fatalf("active = %d, want 2", got)
	}

	sessions.close(name, SessionOptions{IdleTimeout: time.Hour}, func(ctx context.Context) error { return nil })
	if got := sessions.states[name].active; got != 1 {
		t.Fatalf("active after first close = %d, want 1", got)
	}

	sessions.close(name, SessionOptions{IdleTimeout: time.Hour}, func(ctx context.Context) error { return nil })
	state := sessions.states[name]
	if state == nil || state.active != 0 || state.timer == nil {
		t.Fatalf("state after final close = %#v, want inactive with idle timer", state)
	}
}

func TestSandboxSessionCloseWithoutIdleTimeoutDropsState(t *testing.T) {
	t.Parallel()
	sessions := &sandboxSessions{states: map[string]*sandboxSessionState{}}
	name := "ctgbot-session-no-timeout"

	sessions.begin(name)
	sessions.close(name, SessionOptions{}, nil)
	if _, ok := sessions.states[name]; ok {
		t.Fatalf("expected session state to be removed")
	}
}

func TestSandboxSessionIdleTimerStopsAndDropsState(t *testing.T) {
	t.Parallel()
	sessions := &sandboxSessions{states: map[string]*sandboxSessionState{}}
	name := "ctgbot-session-timeout"
	stopped := make(chan struct{}, 1)

	sessions.begin(name)
	sessions.close(name, SessionOptions{IdleTimeout: time.Millisecond}, func(ctx context.Context) error {
		stopped <- struct{}{}
		return nil
	})

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("idle stop did not run")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, ok := sessions.states[name]; !ok {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("expected session state to be removed after idle stop")
}
