package codex

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
)

type stubExecutor struct {
	result     TurnResult
	err        error
	lastPrompt string
	calls      int
}

func (e *stubExecutor) RunTurn(ctx context.Context, runtime ExecRuntime, output OutputHandler, request TurnRequest) (TurnResult, error) {
	_, _, _ = ctx, runtime, output
	e.calls++
	e.lastPrompt = request.Prompt
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
func (stubTurnRuntime) Send(ctx context.Context, payload messenger.OutboundPayload) error {
	_, _ = ctx, payload
	return nil
}
func (stubTurnRuntime) StartChatAction(ctx context.Context, action messenger.ChatAction) (func(), error) {
	_, _ = ctx, action
	return func() {}, nil
}
func (stubTurnRuntime) WorkspacePath() string { return "/tmp/workspace" }
func (stubTurnRuntime) ComponentHome(componentID modeluuid.UUID) (v5runtime.Home, bool) {
	_ = componentID
	return v5runtime.Home{}, false
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
				ID:          modeluuid.New(),
				ChatID:      modeluuid.New(),
				KeepRunning: false,
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
				ID:          modeluuid.New(),
				ChatID:      modeluuid.New(),
				KeepRunning: true,
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
				ID:          modeluuid.New(),
				ChatID:      modeluuid.New(),
				KeepRunning: false,
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

func TestCodexBootstrapIncludesTurnInstructions(t *testing.T) {
	text, err := codexBootstrap("/workspace", "/profile/components/codex/codex", component.TurnInstructions{
		ChatProvider:              "Telegram",
		MessagePrefix:             "🤖",
		KeepRepliesConcise:        true,
		HostbridgeCommandNames:    []string{"docker", "git-push-ctgbot"},
		HostbridgeControlCommands: []string{"hostbridge codex status", "hostbridge config list"},
	})
	if err != nil {
		t.Fatalf("codexBootstrap() error = %v", err)
	}
	for _, want := range []string{
		"The `hostbridge` command is available",
		"Canonical hostbridge control commands for this chat:",
		"`hostbridge codex status`",
		"`hostbridge config list`",
		"Available hostbridge run aliases: `docker, git-push-ctgbot`",
		"The user interacts through Telegram; keep replies concise",
		"Start every assistant message with `🤖`",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("bootstrap missing %q:\n%s", want, text)
		}
	}
}
