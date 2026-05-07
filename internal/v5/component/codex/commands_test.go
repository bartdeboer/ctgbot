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
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
	"github.com/bartdeboer/go-clistate"
)

type testRuntime struct {
	status       v5runtime.Status
	refreshCalls int
	stopCalls    int
	stopErr      error
}

func (r *testRuntime) Kind() string { return "docker" }
func (r *testRuntime) ComponentHome() v5runtime.Home {
	return v5runtime.Home{Path: "/tmp/codex-home"}
}
func (r *testRuntime) RuntimeComponentHomePath() string {
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
func (r *testRuntime) Start(ctx context.Context, workspacePath string, threadID modeluuid.UUID) (v5runtime.Status, error) {
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
func (r *testRuntime) Status(ctx context.Context, workspacePath string, threadID modeluuid.UUID) (v5runtime.Status, error) {
	_, _, _ = ctx, workspacePath, threadID
	return r.status, nil
}
func (r *testRuntime) Exec(ctx context.Context, workspacePath string, threadID modeluuid.UUID, commands commandengine.CommandExecutor, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	_, _, _, _, _, _, _, _ = ctx, workspacePath, threadID, commands, stdout, stderr, name, args
	return nil
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
			status: v5runtime.Status{
				Name:                 "ctgbot-v5-codex-thread",
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

		result, err := engine.Run(ctx, base, []string{"codex", "model", "set", "gpt-test"})
		if err != nil {
			t.Fatalf("model set error = %v", err)
		}
		if got, want := result.Text, "codex model=gpt-test"; got != want {
			t.Fatalf("model set text = %q, want %q", got, want)
		}

		saved, err := storage.Threads().GetByID(ctx, thread.ID)
		if err != nil {
			t.Fatalf("Threads().GetByID() error = %v", err)
		}
		if got := saved.CodexModel; got != "" {
			t.Fatalf("stored legacy thread model = %q, want empty", got)
		}
		stateRow, err := storage.ThreadComponentStates().GetByThreadAndComponent(ctx, thread.ID, registration.ID)
		if err != nil {
			t.Fatalf("ThreadComponentStates().GetByThreadAndComponent() error = %v", err)
		}
		if stateRow == nil {
			t.Fatal("expected thread component state row")
		}
		if got, want := stateRow.StateJSON, `{"model":"gpt-test"}`; got != want {
			t.Fatalf("state json = %q, want %q", got, want)
		}

		statusResult, err := engine.Run(ctx, base, []string{"codex", "status"})
		if err != nil {
			t.Fatalf("status error = %v", err)
		}
		for _, want := range []string{
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

func TestCodexCommandModelStatusFallsBackToLegacyThreadStateUntilTouched(t *testing.T) {
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
		thread := &coremodel.Thread{ID: modeluuid.New(), ChatID: chat.ID, CodexModel: "legacy-model"}
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

		statusResult, err := engine.Run(ctx, base, []string{"codex", "model"})
		if err != nil {
			t.Fatalf("model status error = %v", err)
		}
		for _, want := range []string{
			"codex model: legacy-model",
			"source: legacy_thread",
		} {
			if !strings.Contains(statusResult.Text, want) {
				t.Fatalf("model status missing %q:\n%s", want, statusResult.Text)
			}
		}

		clearResult, err := engine.Run(ctx, base, []string{"codex", "model", "clear"})
		if err != nil {
			t.Fatalf("model clear error = %v", err)
		}
		for _, want := range []string{
			"codex model cleared",
			"source: codex",
		} {
			if !strings.Contains(clearResult.Text, want) {
				t.Fatalf("model clear missing %q:\n%s", want, clearResult.Text)
			}
		}
		saved, err := storage.Threads().GetByID(ctx, thread.ID)
		if err != nil {
			t.Fatalf("Threads().GetByID() error = %v", err)
		}
		if got := saved.CodexModel; got != "" {
			t.Fatalf("legacy thread model = %q, want empty after clear", got)
		}
		stateRow, err := storage.ThreadComponentStates().GetByThreadAndComponent(ctx, thread.ID, registration.ID)
		if err != nil {
			t.Fatalf("ThreadComponentStates().GetByThreadAndComponent() error = %v", err)
		}
		if stateRow != nil {
			t.Fatalf("expected no thread component state row after clear, got %#v", stateRow)
		}
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

		result, err := engine.Run(ctx, base, []string{"codex", "model", "effort", "set", "high"})
		if err != nil {
			t.Fatalf("model effort set error = %v", err)
		}
		if got, want := result.Text, "codex reasoning effort=high"; got != want {
			t.Fatalf("model effort text = %q, want %q", got, want)
		}
		stateRow, err := storage.ThreadComponentStates().GetByThreadAndComponent(ctx, thread.ID, registration.ID)
		if err != nil {
			t.Fatalf("ThreadComponentStates().GetByThreadAndComponent() error = %v", err)
		}
		if stateRow == nil {
			t.Fatal("expected thread component state row")
		}
		if got, want := stateRow.StateJSON, `{"reasoning_effort":"high"}`; got != want {
			t.Fatalf("state json = %q, want %q", got, want)
		}
	})
}

func TestCodexCommandStartAndStopToggleKeepRunning(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		cfg := newTestConfig(t, root)
		storage := repository.NewMemory()
		registration := coremodel.Component{ID: modeluuid.New(), Type: Type, Name: Type}
		runtime := &testRuntime{
			status: v5runtime.Status{
				Name:                 "ctgbot-v5-codex-thread",
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
		if got, want := startResult.Text, "container started\nkeep_running: true\ncontainer: ctgbot-v5-codex-thread\nstate: running"; got != want {
			t.Fatalf("start text = %q, want %q", got, want)
		}
		saved, err := storage.Threads().GetByID(ctx, thread.ID)
		if err != nil {
			t.Fatalf("Threads().GetByID() after start error = %v", err)
		}
		if !saved.KeepRunning {
			t.Fatal("KeepRunning = false, want true after start")
		}

		stopResult, err := engine.Run(ctx, base, []string{"codex", "container", "stop"})
		if err != nil {
			t.Fatalf("stop error = %v", err)
		}
		if got, want := stopResult.Text, "container stopped\nkeep_running: false"; got != want {
			t.Fatalf("stop text = %q, want %q", got, want)
		}
		saved, err = storage.Threads().GetByID(ctx, thread.ID)
		if err != nil {
			t.Fatalf("Threads().GetByID() after stop error = %v", err)
		}
		if saved.KeepRunning {
			t.Fatal("KeepRunning = true, want false after stop")
		}
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
	})
}

func newCodexCommandEngine(t *testing.T, c *Component, source commandengine.Source) *commandengine.Engine {
	t.Helper()
	registry := commandengine.NewRegistry()
	if err := c.RegisterCommandHandlers(registry); err != nil {
		t.Fatalf("RegisterCommandHandlers() error = %v", err)
	}
	router, err := commandengine.NewRouter(c.CommandDefinitions(), source)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	return commandengine.NewEngine(router, registry)
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
