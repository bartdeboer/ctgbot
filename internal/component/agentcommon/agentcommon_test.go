package agentcommon

import (
	"context"
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

func TestThreadResolvesContextThreadIDAndSandboxFallback(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	chat := coremodel.Chat{Enabled: true}
	if err := storage.Chats().Save(ctx, &chat); err != nil {
		t.Fatal(err)
	}
	thread := coremodel.Thread{ChatID: chat.ID, Label: "primary"}
	if err := storage.Threads().Save(ctx, &thread); err != nil {
		t.Fatal(err)
	}
	fallback := coremodel.Thread{ChatID: chat.ID, Label: "sandbox"}
	if err := storage.Threads().Save(ctx, &fallback); err != nil {
		t.Fatal(err)
	}

	got, err := Thread(ctx, storage, commandengine.Request{Context: commandengine.Context{ThreadID: thread.ID, SandboxID: fallback.ID}}, "test")
	if err != nil {
		t.Fatalf("Thread(ThreadID) error = %v", err)
	}
	if got.ID != thread.ID {
		t.Fatalf("Thread(ThreadID) = %s, want %s", got.ID, thread.ID)
	}

	got, err = Thread(ctx, storage, commandengine.Request{Context: commandengine.Context{SandboxID: fallback.ID}}, "test")
	if err != nil {
		t.Fatalf("Thread(SandboxID) error = %v", err)
	}
	if got.ID != fallback.ID {
		t.Fatalf("Thread(SandboxID) = %s, want %s", got.ID, fallback.ID)
	}
}

func TestThreadMissingErrorsAreClear(t *testing.T) {
	_, err := Thread(context.Background(), repository.NewMemory(), commandengine.Request{}, "test")
	if err == nil || !strings.Contains(err.Error(), "missing thread id") {
		t.Fatalf("Thread(missing id) error = %v, want missing thread id", err)
	}

	missingID := modeluuid.New()
	_, err = Thread(context.Background(), repository.NewMemory(), commandengine.Request{Context: commandengine.Context{ThreadID: missingID}}, "test")
	if err == nil || !strings.Contains(err.Error(), "thread not found") {
		t.Fatalf("Thread(missing row) error = %v, want thread not found", err)
	}
}

func TestThreadWorkspaceLoadsChatAndCallsResolver(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	chat := coremodel.Chat{Workspace: "main", Enabled: true}
	if err := storage.Chats().Save(ctx, &chat); err != nil {
		t.Fatal(err)
	}
	thread := coremodel.Thread{ChatID: chat.ID}
	if err := storage.Threads().Save(ctx, &thread); err != nil {
		t.Fatal(err)
	}
	var resolvedChat coremodel.Chat
	gotThread, workspace, err := ThreadWorkspace(ctx, storage, func(_ context.Context, chat coremodel.Chat) (string, error) {
		resolvedChat = chat
		return "/workspace/" + chat.Workspace, nil
	}, commandengine.Request{Context: commandengine.Context{ThreadID: thread.ID}}, "test")
	if err != nil {
		t.Fatalf("ThreadWorkspace() error = %v", err)
	}
	if gotThread.ID != thread.ID || resolvedChat.ID != chat.ID || workspace != "/workspace/main" {
		t.Fatalf("ThreadWorkspace() thread=%s resolvedChat=%s workspace=%q", gotThread.ID, resolvedChat.ID, workspace)
	}

	_, _, err = ThreadWorkspace(ctx, storage, nil, commandengine.Request{Context: commandengine.Context{ThreadID: thread.ID}}, "test")
	if err == nil || !strings.Contains(err.Error(), "missing workspace resolver") {
		t.Fatalf("ThreadWorkspace(nil resolver) error = %v, want missing workspace resolver", err)
	}
}

func TestProviderThreadIDAndBindProviderThreadID(t *testing.T) {
	componentID := modeluuid.New()
	runtime := &fakeTurnRuntime{ids: map[modeluuid.UUID]string{componentID: " provider-thread "}}

	got, err := ProviderThreadID(componentID, runtime)
	if err != nil {
		t.Fatalf("ProviderThreadID() error = %v", err)
	}
	if got != "provider-thread" {
		t.Fatalf("ProviderThreadID() = %q", got)
	}
	if _, err := ProviderThreadID(componentID, nil); err == nil || !strings.Contains(err.Error(), "missing turn runtime") {
		t.Fatalf("ProviderThreadID(nil) error = %v, want missing turn runtime", err)
	}

	if err := BindProviderThreadID(componentID, runtime, " next-thread "); err != nil {
		t.Fatalf("BindProviderThreadID() error = %v", err)
	}
	if runtime.ids[componentID] != "next-thread" {
		t.Fatalf("bound id = %q, want next-thread", runtime.ids[componentID])
	}
	if err := BindProviderThreadID(componentID, runtime, " "); err != nil {
		t.Fatalf("BindProviderThreadID(empty) error = %v", err)
	}
}

func TestJSONStateStoreLoadSaveUpdateDeleteEmpty(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	threadID := modeluuid.New()
	componentID := modeluuid.New()
	store := JSONStateStore[testState]{
		Storage:     storage,
		ComponentID: componentID,
		Label:       "test",
		Clean: func(state testState) testState {
			state.Value = strings.TrimSpace(state.Value)
			return state
		},
		IsZero: func(state testState) bool { return strings.TrimSpace(state.Value) == "" },
	}

	row, state, err := store.Load(ctx, threadID)
	if err != nil {
		t.Fatalf("Load(empty) error = %v", err)
	}
	if row != nil || state.Value != "" {
		t.Fatalf("Load(empty) row=%#v state=%#v, want empty", row, state)
	}
	if err := store.Save(ctx, storage, threadID, nil, testState{Value: " first "}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	row, state, err = store.Load(ctx, threadID)
	if err != nil {
		t.Fatalf("Load(saved) error = %v", err)
	}
	if row == nil || state.Value != "first" {
		t.Fatalf("Load(saved) row=%#v state=%#v, want first", row, state)
	}
	if err := store.Update(ctx, threadID, func(state *testState) { state.Value = " second " }); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	_, state, err = store.Load(ctx, threadID)
	if err != nil {
		t.Fatalf("Load(updated) error = %v", err)
	}
	if state.Value != "second" {
		t.Fatalf("updated state = %#v, want second", state)
	}
	if err := store.Update(ctx, threadID, func(state *testState) { state.Value = " " }); err != nil {
		t.Fatalf("Update(empty) error = %v", err)
	}
	row, _, err = store.Load(ctx, threadID)
	if err != nil {
		t.Fatalf("Load(deleted) error = %v", err)
	}
	if row != nil {
		t.Fatalf("Load(deleted) row=%#v, want nil", row)
	}
}

type testState struct {
	Value string `json:"value,omitempty"`
}

type fakeTurnRuntime struct{ ids map[modeluuid.UUID]string }

func (f *fakeTurnRuntime) Commands() commandengine.CommandExecutor { return nil }
func (f *fakeTurnRuntime) Instructions() component.TurnInstructions {
	return component.TurnInstructions{}
}
func (f *fakeTurnRuntime) Send(context.Context, message.OutboundPayload) error { return nil }
func (f *fakeTurnRuntime) StartChatAction(context.Context, message.ChatAction) (func(), error) {
	return func() {}, nil
}
func (f *fakeTurnRuntime) WorkspacePath() string { return "" }
func (f *fakeTurnRuntime) ComponentHome(modeluuid.UUID) (runtimepkg.Home, bool) {
	return runtimepkg.Home{}, false
}
func (f *fakeTurnRuntime) ComponentThreadID(componentID modeluuid.UUID) (string, bool, error) {
	if f == nil || f.ids == nil {
		return "", false, nil
	}
	value, ok := f.ids[componentID]
	return value, ok, nil
}
func (f *fakeTurnRuntime) BindComponentThreadID(componentID modeluuid.UUID, componentThreadID string) error {
	if f.ids == nil {
		f.ids = map[modeluuid.UUID]string{}
	}
	f.ids[componentID] = strings.TrimSpace(componentThreadID)
	return nil
}
