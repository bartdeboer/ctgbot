package codex

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/buildassets"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clistate"
)

type testRuntime struct {
	componentHome runtimepkg.Home
	runtimeHome   string
	status        runtimepkg.Status
	refreshCalls  int
	stopCalls     int
	stopErr       error
	execCalls     int
	execName      string
	execArgs      []string
}

func (r *testRuntime) Kind() string { return "docker" }
func (r *testRuntime) ComponentHome() runtimepkg.Home {
	if strings.TrimSpace(r.componentHome.Path) != "" {
		return r.componentHome
	}
	return runtimepkg.Home{Path: "/tmp/codex-home"}
}
func (r *testRuntime) RuntimeComponentHomePath() string {
	if strings.TrimSpace(r.runtimeHome) != "" {
		return r.runtimeHome
	}
	return "/profile/components/codex/codex"
}
func (r *testRuntime) RuntimeWorkspacePath(workspacePath string) string {
	_ = workspacePath
	return "/workspace"
}
func (r *testRuntime) Refresh(ctx context.Context, workspacePath string, threadID modeluuid.UUID) error {
	_, _, _ = ctx, workspacePath, threadID
	r.refreshCalls++
	return nil
}
func (r *testRuntime) Start(ctx context.Context, workspacePath string, threadID modeluuid.UUID) (runtimepkg.Status, error) {
	_, _, _ = ctx, workspacePath, threadID
	return r.status, nil
}
func (r *testRuntime) Stop(ctx context.Context, workspacePath string, threadID modeluuid.UUID) error {
	_, _, _ = ctx, workspacePath, threadID
	r.stopCalls++
	return r.stopErr
}
func (r *testRuntime) Interrupt(ctx context.Context, workspacePath string, threadID modeluuid.UUID) (bool, error) {
	_, _, _ = ctx, workspacePath, threadID
	return false, nil
}
func (r *testRuntime) Status(ctx context.Context, workspacePath string, threadID modeluuid.UUID) (runtimepkg.Status, error) {
	_, _, _ = ctx, workspacePath, threadID
	return r.status, nil
}
func (r *testRuntime) Exec(ctx context.Context, workspacePath string, threadID modeluuid.UUID, commands commandengine.CommandExecutor, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	_, _, _, _, _, _, _, _ = ctx, workspacePath, threadID, commands, stdout, stderr, name, args
	r.execCalls++
	r.execName = name
	r.execArgs = append([]string(nil), args...)
	return nil
}
func (r *testRuntime) ExecTTY(ctx context.Context, workspacePath string, threadID modeluuid.UUID, commands commandengine.CommandExecutor, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	return r.Exec(ctx, workspacePath, threadID, commands, stdout, stderr, name, args...)
}
func (r *testRuntime) CombinedOutput(ctx context.Context, workspacePath string, threadID modeluuid.UUID, commands commandengine.CommandExecutor, name string, args ...string) ([]byte, error) {
	_, _, _, _, _, _ = ctx, workspacePath, threadID, commands, name, args
	return nil, nil
}
func (r *testRuntime) OpenHTTPRelayPort(ctx context.Context, workspacePath string, threadID modeluuid.UUID, commands commandengine.CommandExecutor, callbackPort int, callbackTimeout time.Duration) (func(context.Context) error, error) {
	_, _, _, _, _, _ = ctx, workspacePath, threadID, commands, callbackPort, callbackTimeout
	return func(context.Context) error { return nil }, nil
}

func TestCodexCommandModelSetAndStatus(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		cfg := newTestConfig(t, root)
		storage := repository.NewMemory()
		registration := coremodel.Component{ID: modeluuid.New(), Type: Type, Name: Type}
		runtime := &testRuntime{
			status: runtimepkg.Status{
				Name:                 "ctgbot-codex-thread",
				State:                "running",
				RuntimeHomePath:      "/profile/components/codex/codex",
				RuntimeWorkspacePath: "/workspace",
			},
		}
		c := &Component{
			registration: registration,
			runtime:      runtime,
			storage:      storage,
			resolveWorkspace: func(_ context.Context, chat coremodel.Chat) (string, error) {
				_ = chat
				return filepath.Join(root, "workspace"), nil
			},
			config: cfg,
		}

		chat := &coremodel.Chat{ID: modeluuid.New(), Label: "team", Enabled: true}
		if err := storage.Chats().Save(ctx, chat); err != nil {
			t.Fatalf("Chats().Save() error = %v", err)
		}
		thread := &coremodel.Thread{ID: modeluuid.New(), ChatID: chat.ID}
		if err := storage.Threads().Save(ctx, thread); err != nil {
			t.Fatalf("Threads().Save() error = %v", err)
		}

		engine := newCodexCommandEngine(t, c, commandengine.SourceMessage)
		base := commandengine.Request{
			Context: commandengine.Context{
				Source:   commandengine.SourceMessage,
				Actor:    commandengine.Actor{ID: "bart", Roles: []simplerbac.Role{simplerbac.RoleUser}},
				ChatID:   chat.ID,
				ThreadID: thread.ID,
			},
		}

		result, err := engine.Run(ctx, base, []string{"codex", "config", "set", "model", "gpt-test"})
		if err != nil {
			t.Fatalf("model set error = %v", err)
		}
		if got, want := result.Text, "model=gpt-test"; got != want {
			t.Fatalf("model set text = %q, want %q", got, want)
		}
		assertThreadState(t, c, ctx, thread.ID, threadState{Model: "gpt-test"})

		statusResult, err := engine.Run(ctx, base, []string{"codex", "status"})
		if err != nil {
			t.Fatalf("status error = %v", err)
		}
		for _, want := range []string{
			"ctgbot_version: " + buildassets.Version(),
			"container_state: running",
			"keep_running: false",
			"codex_model: gpt-test",
			"codex_model_source: thread_component_state",
			"provider_thread_id: (none)",
		} {
			if !strings.Contains(statusResult.Text, want) {
				t.Fatalf("status missing %q:\n%s", want, statusResult.Text)
			}
		}
	})
}

func TestCodexConfigSurfaceCommands(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		cfg := newTestConfig(t, root)
		storage := repository.NewMemory()
		registration := coremodel.Component{ID: modeluuid.New(), Type: Type, Name: Type}
		c := &Component{
			registration: registration,
			runtime:      &testRuntime{},
			storage:      storage,
			resolveWorkspace: func(_ context.Context, chat coremodel.Chat) (string, error) {
				_ = chat
				return filepath.Join(root, "workspace"), nil
			},
			config: cfg,
		}

		chat := &coremodel.Chat{ID: modeluuid.New(), Label: "team", Enabled: true}
		if err := storage.Chats().Save(ctx, chat); err != nil {
			t.Fatalf("Chats().Save() error = %v", err)
		}
		thread := &coremodel.Thread{ID: modeluuid.New(), ChatID: chat.ID}
		if err := storage.Threads().Save(ctx, thread); err != nil {
			t.Fatalf("Threads().Save() error = %v", err)
		}

		engine := newCodexCommandEngine(t, c, commandengine.SourceMessage)
		base := commandengine.Request{
			Context: commandengine.Context{
				Source:   commandengine.SourceMessage,
				Actor:    commandengine.Actor{ID: "bart", Roles: []simplerbac.Role{simplerbac.RoleUser}},
				ChatID:   chat.ID,
				ThreadID: thread.ID,
			},
		}

		list, err := engine.Run(ctx, base, []string{"codex", "config", "list"})
		if err != nil {
			t.Fatalf("codex config list error = %v", err)
		}
		for _, want := range []string{
			"model=",
			"Codex model for this thread",
			"options: gpt-5.5, gpt-5.4",
			"effort=",
			"options: low, medium, high, xhigh",
			"container.keep-running=false",
			"Keep the Codex runtime container running between turns",
		} {
			if !strings.Contains(list.Text, want) {
				t.Fatalf("config list missing %q:\n%s", want, list.Text)
			}
		}

		set, err := engine.Run(ctx, base, []string{"codex", "config", "set", "model", "gpt-test"})
		if err != nil {
			t.Fatalf("codex config set model error = %v", err)
		}
		if got, want := strings.TrimSpace(set.Text), "model=gpt-test"; got != want {
			t.Fatalf("set result = %q, want %q", got, want)
		}
		assertThreadState(t, c, ctx, thread.ID, threadState{Model: "gpt-test"})

		get, err := engine.Run(ctx, base, []string{"codex", "config", "get", "model"})
		if err != nil {
			t.Fatalf("codex config get model error = %v", err)
		}
		for _, want := range []string{
			"model=gpt-test",
			"type: string",
			"options: gpt-5.5, gpt-5.4",
			"writable: true",
		} {
			if !strings.Contains(get.Text, want) {
				t.Fatalf("config get missing %q:\n%s", want, get.Text)
			}
		}

		if _, err := engine.Run(ctx, base, []string{"codex", "config", "set", "effort", "high"}); err != nil {
			t.Fatalf("codex config set effort error = %v", err)
		}
		assertThreadState(t, c, ctx, thread.ID, threadState{Model: "gpt-test", ReasoningEffort: "high"})

		if _, err := engine.Run(ctx, base, []string{"codex", "config", "set", "container.keep-running", "true"}); err != nil {
			t.Fatalf("codex config set container.keep-running error = %v", err)
		}
		keepRunning := true
		assertThreadState(t, c, ctx, thread.ID, threadState{Model: "gpt-test", ReasoningEffort: "high", KeepRunning: &keepRunning})

		if _, err := engine.Run(ctx, base, []string{"codex", "config", "unset", "container.keep-running"}); err != nil {
			t.Fatalf("codex config unset container.keep-running error = %v", err)
		}
		assertThreadState(t, c, ctx, thread.ID, threadState{Model: "gpt-test", ReasoningEffort: "high"})

		unset, err := engine.Run(ctx, base, []string{"codex", "config", "unset", "model"})
		if err != nil {
			t.Fatalf("codex config unset model error = %v", err)
		}
		if !strings.HasPrefix(unset.Text, "model=") {
			t.Fatalf("unset result = %q, want model fallback", unset.Text)
		}
		assertThreadState(t, c, ctx, thread.ID, threadState{ReasoningEffort: "high"})
	})
}

func TestCodexCommandModelClearRemovesThreadComponentState(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		cfg := newTestConfig(t, root)
		storage := repository.NewMemory()
		registration := coremodel.Component{ID: modeluuid.New(), Type: Type, Name: Type}
		c := &Component{
			registration: registration,
			runtime:      &testRuntime{},
			storage:      storage,
			resolveWorkspace: func(_ context.Context, chat coremodel.Chat) (string, error) {
				_ = chat
				return filepath.Join(root, "workspace"), nil
			},
			config: cfg,
		}

		chat := &coremodel.Chat{ID: modeluuid.New(), Label: "team", Enabled: true}
		if err := storage.Chats().Save(ctx, chat); err != nil {
			t.Fatalf("Chats().Save() error = %v", err)
		}
		thread := &coremodel.Thread{ID: modeluuid.New(), ChatID: chat.ID}
		if err := storage.Threads().Save(ctx, thread); err != nil {
			t.Fatalf("Threads().Save() error = %v", err)
		}
		if err := storage.ThreadComponentStates().Save(ctx, &coremodel.ThreadComponentState{
			ThreadID:    thread.ID,
			ComponentID: registration.ID,
			StateJSON:   `{"model":"legacy-model"}`,
		}); err != nil {
			t.Fatalf("ThreadComponentStates().Save() error = %v", err)
		}

		engine := newCodexCommandEngine(t, c, commandengine.SourceMessage)
		base := commandengine.Request{
			Context: commandengine.Context{
				Source:   commandengine.SourceMessage,
				Actor:    commandengine.Actor{ID: "bart", Roles: []simplerbac.Role{simplerbac.RoleUser}},
				ChatID:   chat.ID,
				ThreadID: thread.ID,
			},
		}

		statusResult, err := engine.Run(ctx, base, []string{"codex", "config", "get", "model"})
		if err != nil {
			t.Fatalf("model status error = %v", err)
		}
		for _, want := range []string{
			"model=legacy-model",
			"writable: true",
		} {
			if !strings.Contains(statusResult.Text, want) {
				t.Fatalf("model status missing %q:\n%s", want, statusResult.Text)
			}
		}

		clearResult, err := engine.Run(ctx, base, []string{"codex", "config", "unset", "model"})
		if err != nil {
			t.Fatalf("model clear error = %v", err)
		}
		for _, want := range []string{
			"model=",
		} {
			if !strings.Contains(clearResult.Text, want) {
				t.Fatalf("model clear missing %q:\n%s", want, clearResult.Text)
			}
		}
		assertNoThreadState(t, c, ctx, thread.ID)
	})
}

func TestCodexCommandModelEffortSetUsesThreadComponentState(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		cfg := newTestConfig(t, root)
		storage := repository.NewMemory()
		registration := coremodel.Component{ID: modeluuid.New(), Type: Type, Name: Type}
		c := &Component{
			registration: registration,
			runtime:      &testRuntime{},
			storage:      storage,
			resolveWorkspace: func(_ context.Context, chat coremodel.Chat) (string, error) {
				_ = chat
				return filepath.Join(root, "workspace"), nil
			},
			config: cfg,
		}

		chat := &coremodel.Chat{ID: modeluuid.New(), Label: "team", Enabled: true}
		if err := storage.Chats().Save(ctx, chat); err != nil {
			t.Fatalf("Chats().Save() error = %v", err)
		}
		thread := &coremodel.Thread{ID: modeluuid.New(), ChatID: chat.ID}
		if err := storage.Threads().Save(ctx, thread); err != nil {
			t.Fatalf("Threads().Save() error = %v", err)
		}

		engine := newCodexCommandEngine(t, c, commandengine.SourceMessage)
		base := commandengine.Request{
			Context: commandengine.Context{
				Source:   commandengine.SourceMessage,
				Actor:    commandengine.Actor{ID: "bart", Roles: []simplerbac.Role{simplerbac.RoleUser}},
				ChatID:   chat.ID,
				ThreadID: thread.ID,
			},
		}

		result, err := engine.Run(ctx, base, []string{"codex", "config", "set", "effort", "high"})
		if err != nil {
			t.Fatalf("model effort set error = %v", err)
		}
		if got, want := result.Text, "effort=high"; got != want {
			t.Fatalf("model effort text = %q, want %q", got, want)
		}
		assertThreadState(t, c, ctx, thread.ID, threadState{ReasoningEffort: "high"})
	})
}

func TestCodexCommandStartAndStopToggleKeepRunning(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		cfg := newTestConfig(t, root)
		storage := repository.NewMemory()
		registration := coremodel.Component{ID: modeluuid.New(), Type: Type, Name: Type}
		runtime := &testRuntime{
			status: runtimepkg.Status{
				Name:                 "ctgbot-codex-thread",
				State:                "running",
				RuntimeHomePath:      "/profile/components/codex/codex",
				RuntimeWorkspacePath: "/workspace",
			},
		}
		c := &Component{
			registration: registration,
			runtime:      runtime,
			storage:      storage,
			resolveWorkspace: func(_ context.Context, chat coremodel.Chat) (string, error) {
				_ = chat
				return filepath.Join(root, "workspace"), nil
			},
			config: cfg,
		}

		chat := &coremodel.Chat{ID: modeluuid.New(), Label: "team", Enabled: true}
		if err := storage.Chats().Save(ctx, chat); err != nil {
			t.Fatalf("Chats().Save() error = %v", err)
		}
		thread := &coremodel.Thread{ID: modeluuid.New(), ChatID: chat.ID}
		if err := storage.Threads().Save(ctx, thread); err != nil {
			t.Fatalf("Threads().Save() error = %v", err)
		}

		engine := newCodexCommandEngine(t, c, commandengine.SourceMessage)
		base := commandengine.Request{
			Context: commandengine.Context{
				Source:   commandengine.SourceMessage,
				Actor:    commandengine.Actor{ID: "bart", Roles: []simplerbac.Role{simplerbac.RoleUser}},
				ChatID:   chat.ID,
				ThreadID: thread.ID,
			},
		}

		startResult, err := engine.Run(ctx, base, []string{"codex", "container", "start"})
		if err != nil {
			t.Fatalf("start error = %v", err)
		}
		if got, want := startResult.Text, "container started\nkeep_running: true\ncontainer: ctgbot-codex-thread\nstate: running"; got != want {
			t.Fatalf("start text = %q, want %q", got, want)
		}
		assertThreadState(t, c, ctx, thread.ID, threadState{KeepRunning: boolPtr(true)})

		stopResult, err := engine.Run(ctx, base, []string{"codex", "container", "stop"})
		if err != nil {
			t.Fatalf("stop error = %v", err)
		}
		if got, want := stopResult.Text, "container stopped\nkeep_running: false"; got != want {
			t.Fatalf("stop text = %q, want %q", got, want)
		}
		assertNoThreadState(t, c, ctx, thread.ID)
		if got, want := runtime.stopCalls, 1; got != want {
			t.Fatalf("stop calls = %d, want %d", got, want)
		}
	})
}

func TestCodexCommandPurgeClearsProviderThreadMapping(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		cfg := newTestConfig(t, root)
		storage := repository.NewMemory()
		registration := coremodel.Component{ID: modeluuid.New(), Type: Type, Name: Type}
		runtime := &testRuntime{}
		c := &Component{
			registration: registration,
			runtime:      runtime,
			storage:      storage,
			resolveWorkspace: func(_ context.Context, chat coremodel.Chat) (string, error) {
				_ = chat
				return filepath.Join(root, "workspace"), nil
			},
			config: cfg,
		}

		chat := &coremodel.Chat{ID: modeluuid.New(), Label: "team", Enabled: true}
		if err := storage.Chats().Save(ctx, chat); err != nil {
			t.Fatalf("Chats().Save() error = %v", err)
		}
		thread := &coremodel.Thread{ID: modeluuid.New(), ChatID: chat.ID}
		if err := storage.Threads().Save(ctx, thread); err != nil {
			t.Fatalf("Threads().Save() error = %v", err)
		}
		if err := storage.ThreadComponentMappings().Save(ctx, &coremodel.ThreadComponentMapping{
			ThreadID:          thread.ID,
			ChatID:            chat.ID,
			ComponentID:       registration.ID,
			ComponentThreadID: "provider-thread-1",
		}); err != nil {
			t.Fatalf("ThreadComponentMappings().Save() error = %v", err)
		}
		if err := storage.ThreadComponentStates().Save(ctx, &coremodel.ThreadComponentState{
			ThreadID:    thread.ID,
			ComponentID: registration.ID,
			StateJSON:   `{"keep_running":true}`,
		}); err != nil {
			t.Fatalf("ThreadComponentStates().Save() error = %v", err)
		}

		engine := newCodexCommandEngine(t, c, commandengine.SourceMessage)
		base := commandengine.Request{
			Context: commandengine.Context{
				Source:   commandengine.SourceMessage,
				Actor:    commandengine.Actor{ID: "bart", Roles: []simplerbac.Role{simplerbac.RoleUser}},
				ChatID:   chat.ID,
				ThreadID: thread.ID,
			},
		}

		result, err := engine.Run(ctx, base, []string{"codex", "purge"})
		if err != nil {
			t.Fatalf("purge error = %v", err)
		}
		if got, want := result.Text, "conversation purged"; got != want {
			t.Fatalf("purge text = %q, want %q", got, want)
		}
		if got, want := runtime.refreshCalls, 1; got != want {
			t.Fatalf("refresh calls = %d, want %d", got, want)
		}
		mapping, err := storage.ThreadComponentMappings().GetByThreadAndComponent(ctx, thread.ID, registration.ID)
		if err != nil {
			t.Fatalf("GetByThreadAndComponent() error = %v", err)
		}
		if mapping != nil {
			t.Fatalf("expected mapping to be cleared, got %#v", mapping)
		}
		assertNoThreadState(t, c, ctx, thread.ID)
	})
}

func newCodexCommandEngine(t *testing.T, c *Component, source commandengine.Source) *commandengine.Engine {
	t.Helper()
	engine, err := commandset.NewBoundEngineForSource(source, []commandset.BoundSurface{{
		Surface:       c,
		ComponentRef:  c.registration.Ref(),
		ComponentType: c.registration.Type,
	}})
	if err != nil {
		t.Fatalf("NewBoundEngineForSource() error = %v", err)
	}
	return engine
}

func newTestConfig(t *testing.T, root string) *appstate.Config {
	t.Helper()
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("NewCwd() error = %v", err)
	}
	return appstate.New(root, store)
}

func withTempCwd(t *testing.T, fn func(root string)) {
	t.Helper()
	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir(%q) error = %v", root, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
	fn(root)
}

func assertThreadState(t *testing.T, c *Component, ctx context.Context, threadID modeluuid.UUID, want threadState) {
	t.Helper()
	row, got, err := c.loadThreadState(ctx, threadID)
	if err != nil {
		t.Fatalf("loadThreadState() error = %v", err)
	}
	if row == nil {
		t.Fatal("expected thread component state row")
	}
	got = got.clean()
	want = want.clean()
	if boolValue(got.KeepRunning) != boolValue(want.KeepRunning) || got.Model != want.Model || got.ReasoningEffort != want.ReasoningEffort {
		t.Fatalf("thread state = %#v, want %#v", got, want)
	}
}

func assertNoThreadState(t *testing.T, c *Component, ctx context.Context, threadID modeluuid.UUID) {
	t.Helper()
	row, got, err := c.loadThreadState(ctx, threadID)
	if err != nil {
		t.Fatalf("loadThreadState() error = %v", err)
	}
	if row != nil || !got.isZero() {
		t.Fatalf("expected no thread component state, got row=%#v state=%#v", row, got)
	}
}

func boolValue(v *bool) bool {
	return v != nil && *v
}
