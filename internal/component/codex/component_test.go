package codex

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

type stubExecutor struct {
	result      TurnResult
	err         error
	lastPrompt  string
	lastRequest TurnRequest
	calls       int
}

func (e *stubExecutor) RunTurn(ctx context.Context, runtime ExecRuntime, output OutputHandler, request TurnRequest) (TurnResult, error) {
	_, _, _ = ctx, runtime, output
	e.calls++
	e.lastPrompt = request.Prompt
	e.lastRequest = request
	return e.result, e.err
}

type stubTurnRuntime struct{}

func (stubTurnRuntime) Commands() commandengine.CommandExecutor { return nil }
func (stubTurnRuntime) Instructions() component.TurnInstructions {
	return component.TurnInstructions{
		ChatProvider:              "Telegram",
		MessagePrefix:             "🤖",
		KeepRepliesConcise:        true,
		HostbridgeCommandNames:    []string{"docker", "git-push-ctgbot"},
		HostbridgeControlCommands: []string{"hostbridge codex status", "hostbridge config list"},
	}
}
func (stubTurnRuntime) Send(ctx context.Context, payload message.OutboundPayload) error {
	_, _ = ctx, payload
	return nil
}
func (stubTurnRuntime) StartChatAction(ctx context.Context, action message.ChatAction) (func(), error) {
	_, _ = ctx, action
	return func() {}, nil
}
func (stubTurnRuntime) WorkspacePath() string { return "/tmp/workspace" }
func (stubTurnRuntime) ComponentHome(componentID modeluuid.UUID) (runtimepkg.Home, bool) {
	_ = componentID
	return runtimepkg.Home{}, false
}
func (stubTurnRuntime) ComponentThreadID(componentID modeluuid.UUID) (string, bool, error) {
	_ = componentID
	return "", false, nil
}
func (stubTurnRuntime) BindComponentThreadID(componentID modeluuid.UUID, componentThreadID string) error {
	_, _ = componentID, componentThreadID
	return nil
}

func TestHandleTurnStopsRuntimeWhenKeepRunningDisabled(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		cfg := newTestConfig(t, root)
		storage := repository.NewMemory()
		runtime := &testRuntime{}
		executor := &stubExecutor{
			result: TurnResult{
				Reply:            "done",
				ProviderThreadID: "provider-thread-1",
			},
		}
		registration := coremodel.Component{ID: modeluuid.New(), Type: Type, Name: Type}
		c := &Component{
			registration: registration,
			runtime:      runtime,
			storage:      storage,
			resolveWorkspace: func(_ context.Context, chat coremodel.Chat) (string, error) {
				_ = chat
				return filepath.Join(root, "workspace"), nil
			},
			config: cfg,
			runner: executor,
		}

		result, err := c.HandleTurn(ctx, component.Turn{
			Chat: coremodel.Chat{ID: modeluuid.New(), Enabled: true},
			Thread: coremodel.Thread{
				ID:     modeluuid.New(),
				ChatID: modeluuid.New(),
			},
			Inbound: coremodel.ThreadMessage{ID: modeluuid.New(), Text: "hello"},
			Runtime: stubTurnRuntime{},
		})
		if err != nil {
			t.Fatalf("HandleTurn() error = %v", err)
		}
		if result == nil || result.Final == nil || result.Final.Text != "done" {
			t.Fatalf("unexpected result = %#v", result)
		}
		if got, want := executor.calls, 1; got != want {
			t.Fatalf("executor calls = %d, want %d", got, want)
		}
		if got, want := runtime.stopCalls, 1; got != want {
			t.Fatalf("stop calls = %d, want %d", got, want)
		}
	})
}

func TestHandleTurnKeepsRuntimeRunningWhenEnabled(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		cfg := newTestConfig(t, root)
		storage := repository.NewMemory()
		runtime := &testRuntime{}
		executor := &stubExecutor{
			result: TurnResult{
				Reply:            "done",
				ProviderThreadID: "provider-thread-1",
			},
		}
		registration := coremodel.Component{ID: modeluuid.New(), Type: Type, Name: Type}
		threadID := modeluuid.New()
		if err := storage.ThreadComponentStates().Save(ctx, &coremodel.ThreadComponentState{
			ThreadID:    threadID,
			ComponentID: registration.ID,
			StateJSON:   `{"keep_running":true}`,
		}); err != nil {
			t.Fatalf("ThreadComponentStates().Save() error = %v", err)
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
			runner: executor,
		}

		_, err := c.HandleTurn(ctx, component.Turn{
			Chat: coremodel.Chat{ID: modeluuid.New(), Enabled: true},
			Thread: coremodel.Thread{
				ID:     threadID,
				ChatID: modeluuid.New(),
			},
			Inbound: coremodel.ThreadMessage{ID: modeluuid.New(), Text: "hello"},
			Runtime: stubTurnRuntime{},
		})
		if err != nil {
			t.Fatalf("HandleTurn() error = %v", err)
		}
		if got := runtime.stopCalls; got != 0 {
			t.Fatalf("stop calls = %d, want 0", got)
		}
	})
}

func TestHandleTurnUsesThreadComponentStateOptions(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		cfg := newTestConfig(t, root)
		storage := repository.NewMemory()
		runtime := &testRuntime{}
		executor := &stubExecutor{
			result: TurnResult{
				Reply:            "done",
				ProviderThreadID: "provider-thread-1",
			},
		}
		registration := coremodel.Component{ID: modeluuid.New(), Type: Type, Name: Type}
		threadID := modeluuid.New()
		if err := storage.ThreadComponentStates().Save(ctx, &coremodel.ThreadComponentState{
			ThreadID:    threadID,
			ComponentID: registration.ID,
			StateJSON:   `{"model":"gpt-state","reasoning_effort":"high"}`,
		}); err != nil {
			t.Fatalf("ThreadComponentStates().Save() error = %v", err)
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
			runner: executor,
		}

		_, err := c.HandleTurn(ctx, component.Turn{
			Chat: coremodel.Chat{ID: modeluuid.New(), Enabled: true},
			Thread: coremodel.Thread{
				ID:     threadID,
				ChatID: modeluuid.New(),
			},
			Inbound: coremodel.ThreadMessage{ID: modeluuid.New(), Text: "hello"},
			Runtime: stubTurnRuntime{},
		})
		if err != nil {
			t.Fatalf("HandleTurn() error = %v", err)
		}
		if got, want := executor.lastRequest.Options.Model, "gpt-state"; got != want {
			t.Fatalf("request model = %q, want %q", got, want)
		}
		if got, want := executor.lastRequest.Options.ReasoningEffort, "high"; got != want {
			t.Fatalf("request reasoning effort = %q, want %q", got, want)
		}
	})
}

func TestHandleTurnInjectsRuntimeNoticesIntoBootstrap(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		cfg := newTestConfig(t, root)
		storage := repository.NewMemory()
		homePath := filepath.Join(root, "codex-home")
		runtime := &testRuntime{
			componentHome: runtimepkg.Home{Path: homePath},
			status: runtimepkg.Status{
				RuntimeNotices: []string{"[Runtime notice] container stale"},
			},
		}
		executor := &stubExecutor{result: TurnResult{Reply: "done"}}
		registration := coremodel.Component{ID: modeluuid.New(), Type: Type, Name: Type}
		c := &Component{
			registration: registration,
			runtime:      runtime,
			storage:      storage,
			resolveWorkspace: func(_ context.Context, chat coremodel.Chat) (string, error) {
				_ = chat
				return filepath.Join(root, "workspace"), nil
			},
			config: cfg,
			runner: executor,
		}

		_, err := c.HandleTurn(ctx, component.Turn{
			Chat: coremodel.Chat{ID: modeluuid.New(), Enabled: true},
			Thread: coremodel.Thread{
				ID:     modeluuid.New(),
				ChatID: modeluuid.New(),
			},
			Inbound: coremodel.ThreadMessage{ID: modeluuid.New(), Text: "hello"},
			Runtime: stubTurnRuntime{},
		})
		if err != nil {
			t.Fatalf("HandleTurn() error = %v", err)
		}
		bootstrap, err := os.ReadFile(filepath.Join(homePath, "ctgbot-bootstrap.md"))
		if err != nil {
			t.Fatalf("read bootstrap: %v", err)
		}
		if !strings.Contains(string(bootstrap), "[Runtime notice] container stale") {
			t.Fatalf("bootstrap missing runtime notice:\n%s", string(bootstrap))
		}
	})
}

func TestHandleTurnIgnoresStopFailureAfterSuccessfulReply(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		cfg := newTestConfig(t, root)
		storage := repository.NewMemory()
		runtime := &testRuntime{
			stopErr: fmt.Errorf("stop failed"),
		}
		executor := &stubExecutor{
			result: TurnResult{
				Reply:            "done",
				ProviderThreadID: "provider-thread-1",
			},
		}
		registration := coremodel.Component{ID: modeluuid.New(), Type: Type, Name: Type}
		c := &Component{
			registration: registration,
			runtime:      runtime,
			storage:      storage,
			resolveWorkspace: func(_ context.Context, chat coremodel.Chat) (string, error) {
				_ = chat
				return filepath.Join(root, "workspace"), nil
			},
			config: cfg,
			runner: executor,
		}

		result, err := c.HandleTurn(ctx, component.Turn{
			Chat: coremodel.Chat{ID: modeluuid.New(), Enabled: true},
			Thread: coremodel.Thread{
				ID:     modeluuid.New(),
				ChatID: modeluuid.New(),
			},
			Inbound: coremodel.ThreadMessage{ID: modeluuid.New(), Text: "hello"},
			Runtime: stubTurnRuntime{},
		})
		if err != nil {
			t.Fatalf("HandleTurn() error = %v", err)
		}
		if result == nil || result.Final == nil || result.Final.Text != "done" {
			t.Fatalf("unexpected result = %#v", result)
		}
		if got, want := runtime.stopCalls, 1; got != want {
			t.Fatalf("stop calls = %d, want %d", got, want)
		}
	})
}

func TestAuthStatusRunsComponentScopedLoginStatus(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		cfg := newTestConfig(t, root)
		storage := repository.NewMemory()
		componentHome := filepath.Join(root, ".ctgbot", "components", "codex", "work")
		runtimeHomePath := filepath.Join(root, "runtime-home")
		runtime := &testRuntime{
			componentHome: runtimepkg.Home{Path: componentHome},
			runtimeHome:   runtimeHomePath,
		}
		registration := coremodel.Component{ID: modeluuid.New(), Type: Type, Name: "work"}
		c := &Component{
			registration: registration,
			runtime:      runtime,
			storage:      storage,
			config:       cfg,
		}

		if err := c.AuthStatus(ctx, io.Discard, io.Discard); err != nil {
			t.Fatalf("AuthStatus() error = %v", err)
		}
		if got, want := runtime.execCalls, 1; got != want {
			t.Fatalf("exec calls = %d, want %d", got, want)
		}
		if got, want := runtime.execName, "codex"; got != want {
			t.Fatalf("exec name = %q, want %q", got, want)
		}
		if got, want := strings.Join(runtime.execArgs, " "), "login status"; got != want {
			t.Fatalf("exec args = %q, want %q", got, want)
		}
		if _, err := os.Stat(filepath.Join(componentHome, "config.toml")); err != nil {
			t.Fatalf("expected config.toml: %v", err)
		}
	})
}

func TestCodexBootstrapIncludesTurnInstructions(t *testing.T) {
	text, err := codexBootstrap("/workspace", "/profile/components/codex/codex", component.TurnInstructions{
		ChatProvider:              "Telegram",
		MessagePrefix:             "🤖",
		KeepRepliesConcise:        true,
		HostbridgeCommandNames:    []string{"docker", "git-push-ctgbot"},
		HostbridgeControlCommands: []string{"hostbridge codex status", "hostbridge config list"},
		RuntimeNotices:            []string{"[Runtime notice] image stale"},
	})
	if err != nil {
		t.Fatalf("codexBootstrap() error = %v", err)
	}
	for _, want := range []string{
		"The `hostbridge` command is available",
		"discovering additional hostbridge commands via `hostbridge help`",
		"Canonical hostbridge control commands for this chat:",
		"`hostbridge codex status`",
		"`hostbridge config list`",
		"Available hostbridge run aliases: `docker, git-push-ctgbot`",
		"[Runtime notice] image stale",
		"The user interacts through Telegram; keep replies concise",
		"Start every assistant message with `🤖`",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("bootstrap missing %q:\n%s", want, text)
		}
	}
}

func TestRuntimeImageTargetsUseConfiguredImage(t *testing.T) {
	withTempCwd(t, func(root string) {
		cfg := newTestConfig(t, root)
		if err := cfg.Docker().SetImage("ctgbot-codex:gpu"); err != nil {
			t.Fatalf("SetImage() error = %v", err)
		}
		if err := cfg.Docker().SetDockerfile("cuda.Dockerfile"); err != nil {
			t.Fatalf("SetDockerfile() error = %v", err)
		}
		c := &Component{
			registration: coremodel.Component{Type: Type, Name: "work"},
			config:       cfg,
		}

		targets, err := c.RuntimeImageTargets(context.Background())
		if err != nil {
			t.Fatalf("RuntimeImageTargets() error = %v", err)
		}
		if got, want := len(targets), 1; got != want {
			t.Fatalf("targets = %d, want %d", got, want)
		}
		target := targets[0]
		if target.Ref != "codex/work" || target.Image != "ctgbot-codex:gpu" || target.Dockerfile != "cuda.Dockerfile" {
			t.Fatalf("target = %#v", target)
		}
	})
}

func TestRuntimeImageTargetsSplitDefaultCodexImage(t *testing.T) {
	withTempCwd(t, func(root string) {
		cfg := newTestConfig(t, root)
		c := &Component{
			registration: coremodel.Component{Type: Type, Name: "work"},
			config:       cfg,
		}

		targets, err := c.RuntimeImageTargets(context.Background())
		if err != nil {
			t.Fatalf("RuntimeImageTargets() error = %v", err)
		}
		if got, want := len(targets), 2; got != want {
			t.Fatalf("targets = %d, want %d: %#v", got, want, targets)
		}
		if targets[0].Name != "codex-base" || targets[0].Image != DefaultBaseImage || targets[0].Dockerfile != "codex.base.Dockerfile" {
			t.Fatalf("base target = %#v", targets[0])
		}
		if targets[1].Name != "codex" || targets[1].Image != DefaultImage || targets[1].Dockerfile != "codex.Dockerfile" || !targets[1].NoCache {
			t.Fatalf("component target = %#v", targets[1])
		}
		if len(targets[1].DependsOn) != 1 || targets[1].DependsOn[0] != "codex-base" {
			t.Fatalf("component dependencies = %#v", targets[1].DependsOn)
		}
	})
}
