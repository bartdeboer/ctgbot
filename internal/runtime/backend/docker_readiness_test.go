package backend

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/containerengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

func dockerReadinessFixture(t *testing.T, script string, handler http.HandlerFunc) *Runtime {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\n"+script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &Runtime{registration: coremodel.Component{Type: "llamacpp", Name: "test"},
		containers: containerengine.NewManager(nil), service: ServiceSpec{HealthURL: server.URL}}
}

func TestDockerReadinessTerminalStateBeatsHealthyListener(t *testing.T) {
	for _, state := range []string{"exited", "dead", "missing"} {
		t.Run(state, func(t *testing.T) {
			script := "echo " + state
			if state == "missing" {
				script = "echo 'Error: No such container: test' >&2; exit 1"
			}
			var hits atomic.Int32
			r := dockerReadinessFixture(t, script, func(w http.ResponseWriter, _ *http.Request) { hits.Add(1) })
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			err := r.waitReady(ctx)
			if err == nil || !strings.Contains(err.Error(), "is "+state) || !strings.Contains(err.Error(), "docker logs --tail 50 "+r.containerName()) {
				t.Fatalf("err=%v", err)
			}
			if hits.Load() != 0 {
				t.Fatal("terminal container was health-probed")
			}
		})
	}
}

func TestDockerReadinessDetectsExitDuringProbe(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state")
	t.Setenv("READINESS_STATE", stateFile)
	if err := os.WriteFile(stateFile, []byte("running"), 0600); err != nil {
		t.Fatal(err)
	}
	r := dockerReadinessFixture(t, `cat "$READINESS_STATE"`, func(w http.ResponseWriter, _ *http.Request) {
		if err := os.WriteFile(stateFile, []byte("exited"), 0600); err != nil {
			t.Error(err)
		}
	})
	err := r.waitReady(t.Context())
	if err == nil || !strings.Contains(err.Error(), "is exited") {
		t.Fatalf("err=%v", err)
	}
}

func TestDockerReadinessRetriesLoading(t *testing.T) {
	var hits atomic.Int32
	r := dockerReadinessFixture(t, "echo running", func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(503)
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := r.waitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("hits=%d", hits.Load())
	}
}

func TestDockerReadinessCancellationAndDeadline(t *testing.T) {
	for _, cancelNow := range []bool{false, true} {
		t.Run(map[bool]string{false: "deadline", true: "cancel"}[cancelNow], func(t *testing.T) {
			r := dockerReadinessFixture(t, "echo running", func(w http.ResponseWriter, req *http.Request) { <-req.Context().Done() })
			ctx, cancel := context.WithTimeout(t.Context(), 80*time.Millisecond)
			defer cancel()
			want := context.DeadlineExceeded
			if cancelNow {
				cancel()
				want = context.Canceled
			}
			if err := r.waitReady(ctx); !errors.Is(err, want) {
				t.Fatalf("err=%v want=%v", err, want)
			}
		})
	}
}

func TestDockerReadinessInspectFailure(t *testing.T) {
	r := dockerReadinessFixture(t, "echo 'daemon unavailable' >&2; exit 1", func(http.ResponseWriter, *http.Request) {})
	if err := r.waitReady(t.Context()); err == nil || !strings.Contains(err.Error(), "daemon unavailable") {
		t.Fatalf("err=%v", err)
	}
}

func TestDockerHealthBoundsBodyAndRejectsRedirect(t *testing.T) {
	for _, mode := range []string{"large", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			var destinationHits atomic.Int32
			destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				destinationHits.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			defer destination.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "redirect" {
					http.Redirect(w, r, destination.URL, http.StatusFound)
					return
				}
				_, _ = w.Write([]byte(strings.Repeat("x", (64<<10)+1)))
			}))
			defer server.Close()
			if err := probeDockerHealth(t.Context(), server.URL); err == nil {
				t.Fatal("expected bounded response failure")
			}
			if hits := destinationHits.Load(); hits != 0 {
				t.Fatalf("redirect destination received %d requests, want zero", hits)
			}
		})
	}
}
