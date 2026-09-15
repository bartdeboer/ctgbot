package copilot

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

type fakeRuntime struct {
	runtimepkg.ThreadRuntime // Any unplanned runtime operation panics in tests.
	profile                  string
	calls                    []string
	args                     []string
	onExec                   func(io.Writer, []string) error
	onStop                   func()
	ttyCalls                 int
}

func (*fakeRuntime) Kind() string { return "docker" }
func (r *fakeRuntime) ComponentProfile() runtimepkg.Profile {
	return runtimepkg.Profile{Path: r.profile}
}
func (r *fakeRuntime) RuntimeComponentProfilePath() string   { return r.profile }
func (*fakeRuntime) RuntimeWorkspacePath(path string) string { return path }
func (*fakeRuntime) Status(context.Context, string, modeluuid.UUID) (runtimepkg.Status, error) {
	return runtimepkg.Status{}, nil
}
func (r *fakeRuntime) Exec(_ context.Context, _ string, _ modeluuid.UUID, _ commandengine.CommandExecutor, out, _ io.Writer, name string, args ...string) error {
	r.calls = append(r.calls, "exec")
	r.args = append([]string{name}, args...)
	if r.onExec != nil {
		return r.onExec(out, r.args)
	}
	return nil
}
func (r *fakeRuntime) Stop(context.Context, string, modeluuid.UUID) error {
	r.calls = append(r.calls, "stop")
	if r.onStop != nil {
		r.onStop()
	}
	return nil
}
func (r *fakeRuntime) ExecTTY(_ context.Context, _ string, _ modeluuid.UUID, _ commandengine.CommandExecutor, _, _ io.Writer, name string, args ...string) error {
	r.ttyCalls++
	r.args = append([]string{name}, args...)
	return nil
}

type fakeFactory struct {
	runtimepkg.Factory
	runtime *fakeRuntime
	config  runtimepkg.BindConfig
	binds   int
}

func (*fakeFactory) Kind() string { return "docker" }
func (f *fakeFactory) RuntimeComponentProfilePath(coremodel.Component, runtimepkg.Profile) string {
	return f.runtime.profile
}
func (f *fakeFactory) Bind(_ coremodel.Component, _ runtimepkg.Profile, cfg runtimepkg.BindConfig) runtimepkg.ThreadRuntime {
	f.binds++
	f.config = cfg
	return f.runtime
}

type fakeTurn struct {
	store     repository.Storage
	thread    coremodel.Thread
	workspace string
	bindings  int
}

func (*fakeTurn) Commands() commandengine.CommandExecutor { return nil }
func (*fakeTurn) Instructions() component.TurnInstructions {
	return component.TurnInstructions{MessagePrefix: "🤖", KeepRepliesConcise: true, HostbridgeControlCommands: []string{"turn info"}, HostbridgeCommandNames: []string{"git"}}
}
func (*fakeTurn) Send(context.Context, message.OutboundPayload) error {
	panic("broker, not provider, relays Final")
}
func (*fakeTurn) StartChatAction(context.Context, message.ChatAction) (func(), error) {
	return func() {}, nil
}
func (r *fakeTurn) WorkspacePath() string { return r.workspace }
func (*fakeTurn) ComponentProfile(modeluuid.UUID) (runtimepkg.Profile, bool) {
	return runtimepkg.Profile{}, false
}
func (r *fakeTurn) ComponentThreadID(id modeluuid.UUID) (string, bool, error) {
	mapping, err := r.store.ThreadComponentMappings().GetByThreadAndComponent(context.Background(), r.thread.ID, id)
	if mapping == nil {
		return "", false, err
	}
	return mapping.ComponentThreadID, true, err
}
func (r *fakeTurn) BindComponentThreadID(id modeluuid.UUID, session string) error {
	r.bindings++
	return r.store.ThreadComponentMappings().Save(context.Background(), &coremodel.ThreadComponentMapping{ComponentID: id, ThreadID: r.thread.ID, ChatID: r.thread.ChatID, ComponentThreadID: session})
}

func newTestComponent(t *testing.T) (*Component, *fakeFactory, *fakeTurn) {
	t.Helper()
	store := repository.NewMemory()
	chat := coremodel.Chat{Enabled: true}
	if err := store.Chats().Save(t.Context(), &chat); err != nil {
		t.Fatal(err)
	}
	thread := coremodel.Thread{ChatID: chat.ID}
	if err := store.Threads().Save(t.Context(), &thread); err != nil {
		t.Fatal(err)
	}
	reg := coremodel.Component{ID: modeluuid.New(), Type: Type, Name: "copilot"}
	factory := &fakeFactory{runtime: &fakeRuntime{profile: t.TempDir()}}
	value, err := New(t.Context(), reg, factory, runtimepkg.Profile{Path: factory.runtime.profile}, store, func(context.Context, coremodel.Chat) (string, error) { return "/workspace", nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	return value.(*Component), factory, &fakeTurn{store: store, thread: thread, workspace: "/workspace"}
}

func selectedID(args []string) (string, bool) {
	for _, a := range args {
		if strings.HasPrefix(a, "--resume=") {
			return strings.TrimPrefix(a, "--resume="), true
		}
		if strings.HasPrefix(a, "--session-id=") {
			return strings.TrimPrefix(a, "--session-id="), false
		}
	}
	return "", false
}

func TestTurnReconstructionIsolationAndCleanup(t *testing.T) {
	c, f, turn := newTestComponent(t)
	run := func(tr *fakeTurn, wantResume bool, output string) (*component.TurnResult, error) {
		f.runtime.calls = nil
		promptPath := ""
		f.runtime.onExec = func(out io.Writer, args []string) error {
			id, resume := selectedID(args)
			if resume != wantResume {
				t.Fatal("resume", args)
			}
			promptPath = args[8]
			body, err := os.ReadFile(promptPath)
			if err != nil || !strings.Contains(string(body), "User request:\nhello") || !strings.Contains(string(body), "hostbridge") {
				t.Fatalf("prompt=%s err=%v", body, err)
			}
			if output == "malformed" {
				io.WriteString(out, "{broken\n")
				return nil
			}
			io.WriteString(out, fullMessage("reply")+terminal(id, 0))
			return nil
		}
		f.runtime.onStop = func() {
			if _, err := os.Stat(promptPath); err != nil {
				t.Fatal("prompt removed before exec cleanup", err)
			}
			if output != "malformed" {
				id, ok, err := tr.ComponentThreadID(c.Registration.ID)
				if err != nil || !ok || id == "" {
					t.Fatal("bind did not precede stop")
				}
			}
		}
		result, err := c.HandleTurn(t.Context(), component.Turn{Thread: tr.thread, Runtime: tr, Prompt: "hello"})
		if !slices.Equal(f.runtime.calls, []string{"exec", "stop"}) {
			t.Fatal(f.runtime.calls)
		}
		if _, statErr := os.Stat(promptPath); !os.IsNotExist(statErr) {
			t.Fatal("staged prompt retained", statErr)
		}
		return result, err
	}
	if r, err := run(turn, false, ""); err != nil || r.Final.Text != "reply" {
		t.Fatal(r, err)
	}
	first, _, _ := turn.ComponentThreadID(c.Registration.ID)
	// New component object, same durable repositories/profile. No in-memory session cache.
	value, err := New(t.Context(), c.Registration, f, runtimepkg.Profile{Path: f.runtime.profile}, turn.store, c.ResolveWorkspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	c = value.(*Component)
	if _, err := run(turn, true, ""); err != nil {
		t.Fatal(err)
	}
	other := coremodel.Thread{ChatID: turn.thread.ChatID}
	if err := turn.store.Threads().Save(t.Context(), &other); err != nil {
		t.Fatal(err)
	}
	second := &fakeTurn{store: turn.store, thread: other, workspace: "/workspace"}
	if _, err := run(second, false, ""); err != nil {
		t.Fatal(err)
	}
	next, _, _ := second.ComponentThreadID(c.Registration.ID)
	if next == first {
		t.Fatal("threads shared session")
	}
	binds := turn.bindings
	if _, err := run(turn, true, "malformed"); err == nil {
		t.Fatal("malformed accepted")
	}
	after, _, _ := turn.ComponentThreadID(c.Registration.ID)
	if after != first || turn.bindings != binds {
		t.Fatal("bad output mutated mapping")
	}
}

func TestFailedTurnIdentityPolicyAndKeepRunning(t *testing.T) {
	c, f, turn := newTestComponent(t)
	keep := true
	if err := c.SetKeepRunning(t.Context(), &turn.thread, &keep); err != nil {
		t.Fatal(err)
	}
	f.runtime.onExec = func(out io.Writer, args []string) error {
		id, _ := selectedID(args)
		io.WriteString(out, terminal(id, 1))
		return fmt.Errorf("provider exit 1")
	}
	_, err := c.HandleTurn(t.Context(), component.Turn{Thread: turn.thread, Runtime: turn, Prompt: "hello"})
	if err == nil || turn.bindings != 1 || !slices.Equal(f.runtime.calls, []string{"exec"}) {
		t.Fatal(err, turn.bindings, f.runtime.calls)
	}
	id, _, _ := turn.ComponentThreadID(c.Registration.ID)
	if _, err := canonicalSessionID(id); err != nil {
		t.Fatal(err)
	}
	value, err := New(t.Context(), c.Registration, f, runtimepkg.Profile{Path: f.runtime.profile}, turn.store, c.ResolveWorkspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := value.(*Component).settings(t.Context(), &turn.thread)
	if err != nil || !settings.KeepRunning {
		t.Fatal(settings, err)
	}
	// Failed/mismatched terminal must not replace even this deliberately retained ID.
	f.runtime.onExec = func(out io.Writer, _ []string) error { io.WriteString(out, terminal(otherID, 1)); return nil }
	_, err = c.HandleTurn(t.Context(), component.Turn{Thread: turn.thread, Runtime: turn, Prompt: "hello"})
	if err == nil || turn.bindings != 1 {
		t.Fatal(err, turn.bindings)
	}
}

func TestAuthStatusDiscoveryAndExplicitLogin(t *testing.T) {
	c, f, _ := newTestComponent(t)
	var out bytes.Buffer
	var nilComponent *Component
	if err := nilComponent.AuthStatus(t.Context(), &out, io.Discard); err != nil || !strings.Contains(out.String(), "unverified") {
		t.Fatal(out.String(), err)
	}
	for _, def := range nilComponent.CommandDefinitions() {
		if def.Pattern == "goal" || def.Pattern == "compact" {
			t.Fatal(def.Pattern)
		}
	}
	if _, err := nilComponent.ConfigSchema(t.Context(), commandengine.Request{}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(f.runtime.profile)
	if err != nil || len(entries) != 0 {
		t.Fatal("discovery modified profile", entries, err)
	}
	if len(f.runtime.calls) != 0 || f.runtime.ttyCalls != 0 {
		t.Fatal("discovery spawned")
	}
	if err := c.Auth(t.Context(), 1455, time.Second, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if f.runtime.ttyCalls != 1 || !slices.Equal(f.runtime.args, []string{"env", "-u", "GH_TOKEN", "-u", "GITHUB_TOKEN", "copilot", "--no-auto-update", "login", "--device-code"}) {
		t.Fatal(f.runtime.args)
	}
}

func TestConfigAndRuntimeBinding(t *testing.T) {
	c, f, turn := newTestComponent(t)
	req := commandengine.Request{Context: commandengine.Context{ThreadID: turn.thread.ID}}
	if err := c.ConfigSet(t.Context(), req, modelKey, "model with 'quotes'"); err != nil {
		t.Fatal(err)
	}
	if value, err := c.ConfigGet(t.Context(), req, modelKey); err != nil || value != "model with 'quotes'" {
		t.Fatal(value, err)
	}
	if err := c.ConfigUnset(t.Context(), req, modelKey); err != nil {
		t.Fatal(err)
	}
	if value, err := c.ConfigGet(t.Context(), req, keepRunningKey); err != nil || value != "false" {
		t.Fatal(value, err)
	}
	if err := os.WriteFile(filepath.Join(f.runtime.profile, "runtime.json"), []byte(`{"env":["GH_TOKEN=unrelated","GITHUB_TOKEN=unrelated","COPILOT_GITHUB_TOKEN=explicit-fixture","OTHER=retained"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(t.Context(), c.Registration, f, runtimepkg.Profile{Path: f.runtime.profile}, turn.store, c.ResolveWorkspace, nil); err != nil {
		t.Fatal(err)
	}
	env := strings.Join(f.config.Env, "\n")
	if strings.Contains(env, "GH_TOKEN=unrelated") || strings.Contains(env, "GITHUB_TOKEN=unrelated") || !strings.Contains(env, "OTHER=retained") || !strings.Contains(env, "COPILOT_HOME="+f.runtime.profile+"/.copilot") {
		t.Fatal("incorrect environment keys")
	}
	if f.config.Image != DefaultImage {
		t.Fatal(f.config)
	}
}

func (r *fakeRuntime) Refresh(context.Context, string, modeluuid.UUID) error {
	r.calls = append(r.calls, "refresh")
	return nil
}
func (r *fakeRuntime) Start(context.Context, string, modeluuid.UUID) (runtimepkg.Status, error) {
	r.calls = append(r.calls, "start")
	return runtimepkg.Status{}, nil
}

func TestLifecycleCommandsPreserveOrPurgeMapping(t *testing.T) {
	c, f, turn := newTestComponent(t)
	if err := turn.BindComponentThreadID(c.Registration.ID, fixtureID); err != nil {
		t.Fatal(err)
	}
	registry := commandengine.NewRegistry()
	if err := c.RegisterCommandHandlers(registry); err != nil {
		t.Fatal(err)
	}
	req := commandengine.Request{Context: commandengine.Context{ThreadID: turn.thread.ID}, CanonicalPattern: "container refresh", Command: agentcommon.RefreshContainer{}}
	if _, err := registry.Execute(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if id, ok, _ := turn.ComponentThreadID(c.Registration.ID); !ok || id != fixtureID {
		t.Fatal("refresh lost conversation")
	}
	req.CanonicalPattern, req.Command = "container start", agentcommon.StartContainer{}
	if _, err := registry.Execute(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if s, err := c.settings(t.Context(), &turn.thread); err != nil || !s.KeepRunning {
		t.Fatal(s, err)
	}
	req.CanonicalPattern, req.Command = "chat purge", agentcommon.PurgeChat{}
	if _, err := registry.Execute(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := turn.ComponentThreadID(c.Registration.ID); ok {
		t.Fatal("purge retained mapping")
	}
	if s, err := c.settings(t.Context(), &turn.thread); err != nil || s.KeepRunning {
		t.Fatal(s, err)
	}
	if !slices.Equal(f.runtime.calls, []string{"refresh", "start", "refresh"}) {
		t.Fatal(f.runtime.calls)
	}
}

func TestInheritedOperatorInterruptAndCallerCancellation(t *testing.T) {
	for _, cancelCaller := range []bool{false, true} {
		c, f, turn := newTestComponent(t)
		f.runtime.onExec = func(io.Writer, []string) error { return context.Canceled }
		ctx, cancel := context.WithCancel(t.Context())
		if cancelCaller {
			cancel()
		}
		result, err := c.HandleTurn(ctx, component.Turn{Thread: turn.thread, Runtime: turn, Prompt: "hello"})
		cancel()
		if result != nil || (err != nil) != cancelCaller || turn.bindings != 0 || !slices.Equal(f.runtime.calls, []string{"exec", "stop"}) {
			t.Fatal(result, err, turn.bindings, f.runtime.calls)
		}
	}
}
