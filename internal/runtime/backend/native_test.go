//go:build darwin || linux

package backend

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

func TestNativeHelper(t *testing.T) {
	if os.Getenv("CTGBOT_NATIVE_HELPER") != "1" {
		return
	}
	if os.Getenv("IGNORE_TERM") == "1" {
		signal.Ignore(syscall.SIGTERM)
	}
	mode := os.Getenv("HELPER_MODE")
	if marker := os.Getenv("TERM_MARKER"); marker != "" {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM)
		go func() { <-signals; os.WriteFile(marker, []byte("graceful"), 0600); os.Exit(0) }()
	}

	if mode == "exit" {
		fmt.Fprintln(os.Stderr, "deliberate early exit")
		os.Exit(17)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if marker := os.Getenv("READY_MARKER"); marker != "" {
			os.WriteFile(marker, []byte("probed"), 0600)
		}
		if mode == "unready" {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, "ok")
	})
	if err := http.ListenAndServe(os.Getenv("HELPER_ADDR"), mux); err != nil {
		os.Exit(18)
	}
	os.Exit(0)
}
func helperSpec(t *testing.T) (runtimepkg.Profile, ServiceSpec) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return runtimepkg.Profile{Path: t.TempDir()}, ServiceSpec{
		Identity: "component/model", BaseURL: "http://" + addr, HealthURL: "http://" + addr + "/health",
		Native: &NativeConfig{Executable: executable, Args: []string{"-test.run=^TestNativeHelper$"}},
		Env:    []string{"CTGBOT_NATIVE_HELPER=1", "HELPER_ADDR=" + addr},
	}
}
func bindHelper(t *testing.T, f *Factory, p runtimepkg.Profile, s ServiceSpec) *nativeRuntime {
	t.Helper()
	r, err := f.BindBackend(coremodel.Component{}, p, runtimepkg.BindConfig{}, s)
	if err != nil {
		t.Fatal(err)
	}
	return r.(*nativeRuntime)
}
func TestNativeRetainedOwnerLifecycle(t *testing.T) {
	f := residentFactory(t)
	defer f.Close(context.Background())
	p, s := helperSpec(t)
	r := bindHelper(t, f, p, s)
	if bindHelper(t, f, p, s) != r {
		t.Fatal("owner not retained")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.Start(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	pid := r.cmd.Process.Pid
	cancel() // Successful startup context must not own the shared process.
	if status, _ := r.Status(context.Background()); status.State != "running" {
		t.Fatal(status)
	}
	if r.cmd.Process.Pid != pid {
		t.Fatal("duplicate process")
	}
	changed := s
	changed.Cmd = []string{"different"}
	if _, err := f.BindBackend(coremodel.Component{}, p, runtimepkg.BindConfig{}, changed); err == nil {
		t.Fatal("spec change accepted")
	}
	if err := bindHelper(t, f, p, s).Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if status, _ := r.Status(context.Background()); status.State != "stopped" {
		t.Fatal(status)
	}
	if _, err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Start(context.Background()); err == nil {
		t.Fatal("start after close")
	}
	if _, err := f.BindBackend(coremodel.Component{}, p, runtimepkg.BindConfig{}, s); err == nil {
		t.Fatal("bind after close")
	}
}
func TestNativeStartupFailureCleanup(t *testing.T) {
	for _, mode := range []string{"exit", "unready"} {
		t.Run(mode, func(t *testing.T) {
			f := residentFactory(t)
			defer f.Close(context.Background())
			p, s := helperSpec(t)
			s.Env = append(s.Env, "HELPER_MODE="+mode)
			r := bindHelper(t, f, p, s)
			ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
			defer cancel()
			if _, err := r.Start(ctx); err == nil {
				t.Fatal("expected failure")
			}
			if r.alive() {
				t.Fatal("failed start leaked child")
			}
		})
	}
}
func TestNativeOccupiedPort(t *testing.T) {
	f := residentFactory(t)
	defer f.Close(context.Background())
	p, s := helperSpec(t)
	listener, err := net.Listen("tcp", s.BaseURL[len("http://"):])
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	r := bindHelper(t, f, p, s)
	if _, err := r.Start(context.Background()); err == nil {
		t.Fatal("adopted occupied port")
	}
	if r.cmd != nil {
		t.Fatal("spawned child on occupied port")
	}
}
func TestNativeCloseCancelsStartup(t *testing.T) {
	f := residentFactory(t)
	p, s := helperSpec(t)
	marker := filepath.Join(p.Path, "ready")
	s.Env = append(s.Env, "HELPER_MODE=unready", "READY_MARKER="+marker)
	r := bindHelper(t, f, p, s)
	done := make(chan struct{})
	go func() { defer close(done); r.Start(context.Background()) }()
	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		select {
		case <-deadline:
			f.Close(context.Background())
			t.Fatal("child never received readiness probe")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := f.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("startup not cancelled")
	}
	select {
	case <-r.done:
	default:
		t.Fatal("in-flight child not reaped")
	}
}
func TestNativeForcedStop(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelled), func(t *testing.T) {
			f := residentFactory(t)
			defer f.Close(context.Background())
			p, s := helperSpec(t)
			s.Env = append(s.Env, "IGNORE_TERM=1")
			r := bindHelper(t, f, p, s)
			if _, err := r.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			cmd := r.cmd
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			if cancelled {
				cancel()
			}
			started := time.Now()
			if err := r.Stop(ctx); err != nil {
				t.Fatal(err)
			}
			if time.Since(started) > 6*time.Second {
				t.Fatal("stop exceeded bound")
			}
			if r.alive() {
				t.Fatal("child survives forced stop")
			}
			status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
			if !ok || status.Signal() != syscall.SIGKILL {
				t.Fatalf("expected direct SIGKILL, got %v", cmd.ProcessState)
			}
		})
	}
}
func TestNativeConfigAndExactArgs(t *testing.T) {
	p := t.TempDir()
	if c, err := LoadNativeConfig(p); err != nil || c != nil {
		t.Fatal(c, err)
	}
	raw := `{"driver":"native","native":{"executable":"/path with spaces/llama","args":["serve","","  exact  "]}}`
	if err := os.WriteFile(filepath.Join(p, "runtime.json"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadNativeConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Args, []string{"serve", "", "  exact  "}) {
		t.Fatal(c.Args)
	}
	for _, raw := range []string{`{"driver":"other"}`, `{"driver":"native"}`, `{"driver":"native","native":{"executable":"llama"}}`, `{"native":{"executable":"/llama"}}`} {
		os.WriteFile(filepath.Join(p, "runtime.json"), []byte(raw), 0600)
		if _, err := LoadNativeConfig(p); err == nil {
			t.Fatal("accepted", raw)
		}
	}
}
func TestNativeLogBound(t *testing.T) {
	var b tailBuffer
	b.Write(make([]byte, 100000))
	b.Write([]byte("tail"))
	if len(b.String()) != 65536 || b.String()[65532:] != "tail" {
		t.Fatal("incorrect bounded tail")
	}
}

func TestNativeIdentityDoesNotNormalizeNames(t *testing.T) {
	f := residentFactory(t)
	defer f.Close(context.Background())
	p, s := helperSpec(t)
	s.Identity = "component/a.b"
	first := bindHelper(t, f, p, s)
	s.Identity = "component/a-b"
	second := bindHelper(t, f, p, s)
	if first == second {
		t.Fatal("distinct identities merged")
	}
}
func TestNativeMissingExecutable(t *testing.T) {
	f := residentFactory(t)
	defer f.Close(context.Background())
	p, s := helperSpec(t)
	s.Native.Executable = filepath.Join(p.Path, "not-installed")
	r := bindHelper(t, f, p, s)
	if _, err := r.Start(context.Background()); err == nil {
		t.Fatal("missing executable accepted")
	}
	if r.cmd != nil {
		t.Fatal("invalid child retained")
	}
}

func residentFactory(t *testing.T) *Factory {
	t.Helper()
	f := New("", nil)
	if err := f.EnableNative(); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestNativeExplicitEmptyRebinding(t *testing.T) {
	for _, args := range [][]string{nil, {}, {"serve"}} {
		f := New("", nil)
		p, spec := helperSpec(t)
		spec.Native.Args = args
		spec.Cmd, spec.Env = []string{}, []string{}
		first := bindHelper(t, f, p, spec)
		if bindHelper(t, f, p, spec) != first {
			t.Fatal("lost owner")
		}
		changed := spec
		changed.Native = &NativeConfig{Executable: spec.Native.Executable, Args: []string{"changed"}}
		if _, err := f.BindBackend(coremodel.Component{}, p, runtimepkg.BindConfig{}, changed); err == nil {
			t.Fatal("accepted changed prefix")
		}
		f.Close(context.Background())
	}
}
func TestUnmanagedNativeLifecycleDenied(t *testing.T) {
	f := New("", nil)
	p, spec := helperSpec(t)
	r := bindHelper(t, f, p, spec)
	if _, err := r.Start(context.Background()); err == nil {
		t.Fatal("unmanaged start")
	}
	if _, err := r.Status(context.Background()); err == nil {
		t.Fatal("unmanaged status")
	}
	if err := r.Stop(context.Background()); err == nil {
		t.Fatal("unmanaged stop")
	}
	if r.cmd != nil {
		t.Fatal("spawned child")
	}
	f.Close(context.Background())
	if err := f.EnableNative(); err == nil {
		t.Fatal("reopened closed factory")
	}
}

func TestNativeGracefulDirectTermination(t *testing.T) {
	f := residentFactory(t)
	defer f.Close(context.Background())
	p, s := helperSpec(t)
	marker := filepath.Join(p.Path, "terminated")
	s.Env = append(s.Env, "TERM_MARKER="+marker)
	r := bindHelper(t, f, p, s)
	if _, err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "graceful" {
		t.Fatalf("TERM handler: %q, %v", data, err)
	}
	select {
	case <-r.done:
	default:
		t.Fatal("direct process not reaped")
	}
}

func TestNativeSpontaneousDirectExitAndRestart(t *testing.T) {
	f := residentFactory(t)
	defer f.Close(context.Background())
	p, s := helperSpec(t)
	r := bindHelper(t, f, p, s)
	if _, err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := r.cmd.Process
	if err := first.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case <-r.done:
	case <-time.After(3 * time.Second):
		t.Fatal("direct process not reaped")
	}
	if status, err := r.Status(context.Background()); err != nil || status.State != "stopped" {
		t.Fatal(status, err)
	}
	if _, err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.cmd.Process == first {
		t.Fatal("reused completed process handle")
	}
	if err := f.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-r.done:
	default:
		t.Fatal("new generation not reaped")
	}
}

func TestNativeStopReportsWaitFailure(t *testing.T) {
	f := residentFactory(t)
	defer f.Close(context.Background())
	p, s := helperSpec(t)
	r := bindHelper(t, f, p, s)
	r.cmd = &exec.Cmd{}
	r.done = make(chan struct{})
	r.exitErr = exec.ErrWaitDelay
	close(r.done)
	if err := r.Stop(context.Background()); !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatal("lost Wait failure", err)
	}
	if _, err := r.Start(context.Background()); !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatal("overwrote failed generation", err)
	}
}
