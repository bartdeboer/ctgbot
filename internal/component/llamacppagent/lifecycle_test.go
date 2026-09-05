package llamacppagent

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

type lifecycleRuntime struct {
	runtimepkg.ThreadRuntime
	dir    string
	run    func(context.Context, string) error
	stops  int
	exited bool
	t      *testing.T
}

func (r *lifecycleRuntime) ComponentProfile() runtimepkg.Profile {
	return runtimepkg.Profile{Path: r.dir}
}
func (r *lifecycleRuntime) RuntimeComponentProfilePath() string  { return r.dir }
func (r *lifecycleRuntime) RuntimeWorkspacePath(p string) string { return p }
func (r *lifecycleRuntime) Exec(ctx context.Context, _ string, _ modeluuid.UUID, _ commandengine.CommandExecutor, _ io.Writer, _ io.Writer, _ string, args ...string) error {
	defer func() { r.exited = true }()
	for i, a := range args {
		if a == "--output" {
			return r.run(ctx, args[i+1])
		}
	}
	return errors.New("missing output flag")
}
func (r *lifecycleRuntime) Stop(ctx context.Context, _ string, _ modeluuid.UUID) error {
	if !r.exited {
		r.t.Error("stopped before Exec joined")
	}
	if ctx.Err() != nil {
		r.t.Error("cleanup inherited cancellation")
	}
	if _, ok := ctx.Deadline(); !ok {
		r.t.Error("cleanup not bounded")
	}
	r.stops++
	// A successful result must already have been read before shutdown.
	paths, _ := filepath.Glob(filepath.Join(r.dir, "toolloop", "turns", "*", "result.json"))
	for _, p := range paths {
		_ = os.Remove(p)
	}
	return nil
}

type lifecycleTurnRuntime struct{ component.TurnRuntime }

func (lifecycleTurnRuntime) WorkspacePath() string { return "/workspace/test" }
func (lifecycleTurnRuntime) Instructions() component.TurnInstructions {
	return component.TurnInstructions{}
}
func (lifecycleTurnRuntime) Commands() commandengine.CommandExecutor { return nil }
func (lifecycleTurnRuntime) StartChatAction(context.Context, message.ChatAction) (func(), error) {
	return func() {}, nil
}
func (lifecycleTurnRuntime) ComponentThreadID(modeluuid.UUID) (string, bool, error) {
	return "", false, nil
}
func (lifecycleTurnRuntime) BindComponentThreadID(modeluuid.UUID, string) error { return nil }

type lifecycleSession struct {
	component.OpenAIChatSession
	closed bool
}

func (*lifecycleSession) BaseURL() string { return "http://localhost:19080" }
func (*lifecycleSession) Model() string   { return "test" }
func (*lifecycleSession) APIKey() string  { return "" }
func (s *lifecycleSession) Close() error  { s.closed = true; return nil }

type lifecycleBackend struct {
	component.OpenAIChatEngine
	session *lifecycleSession
}

func (b *lifecycleBackend) BeginOpenAIChatSession(context.Context, component.InferenceSessionOptions) (component.OpenAIChatSession, error) {
	return b.session, nil
}

type lifecycleResolver struct{ backend *lifecycleBackend }

func (r lifecycleResolver) ResolveComponentRef(_ context.Context, ref string) (*coremodel.Component, error) {
	if ref != "backend" {
		return nil, errors.New("no model profile")
	}
	return &coremodel.Component{}, nil
}
func (r lifecycleResolver) ResolveComponent(context.Context, modeluuid.UUID) (*component.Loaded, error) {
	return &component.Loaded{Component: r.backend}, nil
}

func TestHandleTurnSandboxLifecycle(t *testing.T) {
	for _, mode := range []string{"success", "exec-error", "result-error", "cancel", "keep-running"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			r := &lifecycleRuntime{dir: t.TempDir(), t: t}
			r.run = func(ctx context.Context, path string) error {
				switch mode {
				case "cancel":
					cancel()
					<-ctx.Done()
					return ctx.Err()
				case "exec-error":
					return errors.New("exec failed")
				case "result-error":
					return os.WriteFile(path, []byte("invalid json"), 0600)
				default:
					return os.WriteFile(path, []byte(`{"status":"success","text":"done","conversation_id":"test"}`), 0600)
				}
			}
			session := &lifecycleSession{}
			c := &Component{registration: coremodel.Component{ID: modeluuid.New()}, storage: repository.NewMemory(), runtime: r,
				config: ComponentConfig{Backend: "backend"}, resolver: lifecycleResolver{&lifecycleBackend{session: session}}}
			thread := coremodel.Thread{ID: modeluuid.New()}
			if mode == "keep-running" {
				keep := true
				if err := c.SetThreadSandboxKeepRunning(ctx, component.ThreadSandboxRequest{Thread: thread}, &keep); err != nil {
					t.Fatal(err)
				}
			}
			result, err := c.HandleTurn(ctx, component.Turn{Thread: thread, Inbound: coremodel.ThreadMessage{Text: "test"}, Runtime: lifecycleTurnRuntime{}})
			if mode == "success" || mode == "keep-running" {
				if err != nil || result == nil || result.Final.Text != "done" {
					t.Fatalf("result=%v err=%v", result, err)
				}
			} else if err == nil {
				t.Fatal("expected error")
			}
			wantStops := 1
			if mode == "keep-running" {
				wantStops = 0
			}
			if r.stops != wantStops {
				t.Fatalf("stops=%d want=%d", r.stops, wantStops)
			}
			if !session.closed {
				t.Fatal("backend session not released")
			}
		})
	}
}

func TestSandboxPersistenceIsStoredPerComponentAndThread(t *testing.T) {
	yes, no := true, false
	storage := repository.NewMemory()
	c := &Component{registration: coremodel.Component{ID: modeluuid.New()}, storage: storage}
	req := component.ThreadSandboxRequest{Thread: coremodel.Thread{ID: modeluuid.New()}}
	check := func(c *Component, want bool) {
		t.Helper()
		got, err := c.ThreadSandboxKeepRunning(t.Context(), req)
		if err != nil || got != want {
			t.Fatalf("keep=%v want=%v err=%v", got, want, err)
		}
	}
	check(c, false)
	for _, v := range []*bool{&yes, nil, &yes, &no} {
		if err := c.SetThreadSandboxKeepRunning(t.Context(), req, v); err != nil {
			t.Fatal(err)
		}
		reloaded := &Component{registration: c.registration, storage: storage}
		check(reloaded, v != nil && *v)
	}
	if err := c.SetThreadSandboxKeepRunning(t.Context(), req, &yes); err != nil {
		t.Fatal(err)
	}
	check(&Component{registration: coremodel.Component{ID: modeluuid.New()}, storage: storage}, false)
	other := req
	other.Thread.ID = modeluuid.New()
	if got, err := c.ThreadSandboxKeepRunning(t.Context(), other); err != nil || got {
		t.Fatalf("other thread keep=%v err=%v", got, err)
	}
}
