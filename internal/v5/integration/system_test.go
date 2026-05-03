package integration

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	v5broker "github.com/bartdeboer/ctgbot/internal/v5/broker"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
	v5system "github.com/bartdeboer/ctgbot/internal/v5/system"
	"github.com/bartdeboer/go-clistate"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestV5MockComponentsEndToEnd(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()

		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("NewCwd() error = %v", err)
		}
		if _, err := v5system.SaveProfile(root, store, "test", "local", "profiles/test-root"); err != nil {
			t.Fatalf("SaveProfile() error = %v", err)
		}
		profiles, err := v5system.LoadProfiles(root, store)
		if err != nil {
			t.Fatalf("LoadProfiles() error = %v", err)
		}
		testProfile, ok := profiles["test"]
		if !ok {
			t.Fatal("missing test profile")
		}

		storage := newSQLiteStorage(t)
		runtimeState := &runtimeState{}
		messengerState := &messengerState{
			event: component.InboundEvent{
				ExternalID: "msg-1",
				Payload: messenger.InboundPayload{
					ProviderType:      "mockmsg",
					ProviderChatID:    "chat-1",
					ProviderThreadID:  "provider-thread-1",
					ProviderMessageID: "msg-1",
					UserLabel:         "bart",
					Text:              messenger.TextMessage{Text: "hello"},
				},
			},
		}
		agentState := &agentState{}

		registry := component.NewRegistry()
		if err := registry.Add("mockmsg", func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
			_, _, _, _, _ = ctx, runtime, home, storage, registration
			return &mockMessenger{
				componentID: registration.ID,
				state:       messengerState,
			}, nil
		}); err != nil {
			t.Fatalf("register mockmsg: %v", err)
		}
		if err := registry.Add("mockagent", func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
			_, _, _ = ctx, home, storage
			return &mockAgent{
				componentID: registration.ID,
				runtime:     runtime.Bind(registration, home, "", nil),
				state:       agentState,
			}, nil
		}); err != nil {
			t.Fatalf("register mockagent: %v", err)
		}

		runtimes := map[string]v5runtime.Factory{}
		for name, profile := range profiles {
			runtimes[name] = fakeRuntimeFactory{
				profile: profile,
				rootDir: root,
				state:   runtimeState,
			}
		}
		system := v5system.New(storage, profiles, runtimes, registry)

		messengerRegistration, err := system.EnsureComponent(ctx, "mockmsg", "test")
		if err != nil {
			t.Fatalf("EnsureComponent(mockmsg) error = %v", err)
		}
		agentRegistration, err := system.EnsureComponent(ctx, "mockagent", "test")
		if err != nil {
			t.Fatalf("EnsureComponent(mockagent) error = %v", err)
		}

		if err := system.AuthComponent(ctx, "mockagent", "", "", 0, 0, io.Discard, io.Discard); err != nil {
			t.Fatalf("AuthComponent() error = %v", err)
		}

		authPath := filepath.Join(testProfile.Root, "components", "mockagent", "mockagent", "auth.json")
		if _, err := os.Stat(authPath); err != nil {
			t.Fatalf("auth.json not created at %s: %v", authPath, err)
		}

		chat := &coremodel.Chat{
			Label:   "team",
			Enabled: true,
		}
		if err := storage.Chats().Save(ctx, chat); err != nil {
			t.Fatalf("Chats().Save() error = %v", err)
		}

		if _, err := system.BindChatComponent(ctx, chat.ID, coremodel.ChatComponentRoleSource, messengerRegistration.Ref(), "chat-1"); err != nil {
			t.Fatalf("BindChatComponent(source) error = %v", err)
		}
		if _, err := system.BindChatComponent(ctx, chat.ID, coremodel.ChatComponentRoleRelay, messengerRegistration.Ref(), "chat-1"); err != nil {
			t.Fatalf("BindChatComponent(relay) error = %v", err)
		}
		if _, err := system.BindChatComponent(ctx, chat.ID, coremodel.ChatComponentRoleAgent, agentRegistration.Ref(), ""); err != nil {
			t.Fatalf("BindChatComponent(agent) error = %v", err)
		}

		b := v5broker.New(storage, system, nil)
		if err := b.Run(ctx); err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if messengerState.runCalls != 1 {
			t.Fatalf("run calls = %d, want 1", messengerState.runCalls)
		}
		if agentState.authCalls != 1 {
			t.Fatalf("auth calls = %d, want 1", agentState.authCalls)
		}
		if agentState.turnCalls != 1 {
			t.Fatalf("turn calls = %d, want 1", agentState.turnCalls)
		}
		if agentState.prompt != "hello" {
			t.Fatalf("prompt = %q, want hello", agentState.prompt)
		}
		if runtimeState.execCalls != 1 {
			t.Fatalf("exec calls = %d, want 1", runtimeState.execCalls)
		}
		if runtimeState.lastThreadID.IsNull() {
			t.Fatal("runtime Exec() did not receive a thread id")
		}
		if runtimeState.lastName != "mock-agent" {
			t.Fatalf("exec name = %q, want mock-agent", runtimeState.lastName)
		}
		if got, want := strings.Join(runtimeState.lastArgs, " "), "reply hello"; got != want {
			t.Fatalf("exec args = %q, want %q", got, want)
		}
		if len(messengerState.relayPayloads) != 1 {
			t.Fatalf("relay payload count = %d, want 1", len(messengerState.relayPayloads))
		}
		payload := messengerState.relayPayloads[0]
		if payload.Text.Text != "done" {
			t.Fatalf("relay text = %q, want done", payload.Text.Text)
		}
		if payload.ProviderChatID != "chat-1" {
			t.Fatalf("relay provider chat id = %q, want chat-1", payload.ProviderChatID)
		}
		if payload.ProviderThreadID != "provider-thread-1" {
			t.Fatalf("relay provider thread id = %q, want provider-thread-1", payload.ProviderThreadID)
		}

		messages, err := storage.Messages().ListByThreadID(ctx, runtimeState.lastThreadID)
		if err != nil {
			t.Fatalf("ListByThreadID() error = %v", err)
		}
		if len(messages) != 2 {
			t.Fatalf("stored messages = %d, want 2", len(messages))
		}
	})
}

type runtimeState struct {
	mu           sync.Mutex
	execCalls    int
	lastThreadID modeluuid.UUID
	lastName     string
	lastArgs     []string
}

type fakeRuntimeFactory struct {
	profile v5runtime.Profile
	rootDir string
	state   *runtimeState
}

func (f fakeRuntimeFactory) Kind() string {
	return f.profile.Runtime
}

func (f fakeRuntimeFactory) Profile() v5runtime.Profile {
	return f.profile
}

func (f fakeRuntimeFactory) ComponentHome(registration coremodel.Component) v5runtime.Home {
	return v5runtime.Home{
		HostPath:      filepath.Join(f.profile.Root, "components", registration.Type, registration.Name),
		ContainerPath: "/profile/components/" + registration.Type + "/" + registration.Name,
	}
}

func (f fakeRuntimeFactory) Bind(registration coremodel.Component, home v5runtime.Home, image string, env []string) v5runtime.Runtime {
	_, _, _ = registration, image, env
	return &fakeRuntime{
		rootDir: f.rootDir,
		profile: f.profile,
		home:    home,
		state:   f.state,
	}
}

type fakeRuntime struct {
	rootDir string
	profile v5runtime.Profile
	home    v5runtime.Home
	state   *runtimeState
}

func (r *fakeRuntime) Kind() string {
	return r.profile.Runtime
}

func (r *fakeRuntime) Profile() v5runtime.Profile {
	return r.profile
}

func (r *fakeRuntime) ComponentHome() v5runtime.Home {
	return r.home
}

func (r *fakeRuntime) ThreadWorkspace(threadID modeluuid.UUID) (string, string, error) {
	if threadID.IsNull() {
		return "", "", fmt.Errorf("missing thread id")
	}
	hostPath := filepath.Join(r.rootDir, ".ctgbot", "threads", threadID.String(), "workspace")
	if err := os.MkdirAll(filepath.Join(hostPath, "inbox"), 0o755); err != nil {
		return "", "", err
	}
	return hostPath, "/workspace", nil
}

func (r *fakeRuntime) Exec(ctx context.Context, threadID modeluuid.UUID, commands commandengine.CommandExecutor, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	_, _, _, _ = ctx, commands, stdout, stderr
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	r.state.execCalls++
	r.state.lastThreadID = threadID
	r.state.lastName = name
	r.state.lastArgs = append([]string(nil), args...)
	return nil
}

func (r *fakeRuntime) CombinedOutput(ctx context.Context, threadID modeluuid.UUID, commands commandengine.CommandExecutor, name string, args ...string) ([]byte, error) {
	_, _, _, _, _ = ctx, threadID, commands, name, args
	return []byte("ok"), nil
}

func (r *fakeRuntime) OpenHTTPRelayPort(ctx context.Context, threadID modeluuid.UUID, commands commandengine.CommandExecutor, callbackPort int, callbackTimeout time.Duration) (func(context.Context) error, error) {
	_, _, _, _, _ = ctx, threadID, commands, callbackPort, callbackTimeout
	return func(context.Context) error { return nil }, nil
}

type messengerState struct {
	mu            sync.Mutex
	runCalls      int
	event         component.InboundEvent
	relayPayloads []messenger.OutboundPayload
}

type mockMessenger struct {
	componentID modeluuid.UUID
	state       *messengerState
}

func (m *mockMessenger) Type() string {
	return "mockmsg"
}

func (m *mockMessenger) RunInbound(ctx context.Context, emit component.InboundEmitter) error {
	m.state.mu.Lock()
	m.state.runCalls++
	event := m.state.event
	m.state.mu.Unlock()

	event.ComponentID = m.componentID
	return emit(ctx, event)
}

func (m *mockMessenger) Send(ctx context.Context, payload messenger.OutboundPayload) error {
	_ = ctx
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	m.state.relayPayloads = append(m.state.relayPayloads, payload)
	return nil
}

func (m *mockMessenger) StartChatAction(ctx context.Context, target messenger.ChatTarget, action messenger.ChatAction) (func(), error) {
	_, _, _ = ctx, target, action
	return func() {}, nil
}

type agentState struct {
	mu        sync.Mutex
	authCalls int
	turnCalls int
	prompt    string
}

type mockAgent struct {
	componentID modeluuid.UUID
	runtime     v5runtime.Runtime
	state       *agentState
}

func (a *mockAgent) Type() string {
	return "mockagent"
}

func (a *mockAgent) Auth(ctx context.Context, callbackPort int, callbackTimeout time.Duration, stdout io.Writer, stderr io.Writer) error {
	_, _, _, _, _ = ctx, callbackPort, callbackTimeout, stdout, stderr
	home := a.runtime.ComponentHome()
	if err := os.MkdirAll(home.HostPath, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(home.HostPath, "auth.json"), []byte(`{"ok":true}`), 0o600); err != nil {
		return err
	}
	a.state.mu.Lock()
	a.state.authCalls++
	a.state.mu.Unlock()
	return nil
}

func (a *mockAgent) HandleTurn(ctx context.Context, turn component.Turn) (*component.TurnResult, error) {
	homeFromRuntime := a.runtime.ComponentHome()
	homeFromTurn, ok := turn.Runtime.ComponentHome(a.componentID)
	if !ok {
		return nil, fmt.Errorf("missing component home")
	}
	if homeFromRuntime.HostPath != homeFromTurn.HostPath {
		return nil, fmt.Errorf("component home mismatch: %s != %s", homeFromRuntime.HostPath, homeFromTurn.HostPath)
	}
	if _, err := os.Stat(filepath.Join(homeFromRuntime.HostPath, "auth.json")); err != nil {
		return nil, fmt.Errorf("missing auth.json: %w", err)
	}
	if err := a.runtime.Exec(ctx, turn.Thread.ID, turn.Runtime.Commands(), io.Discard, io.Discard, "mock-agent", "reply", strings.TrimSpace(turn.Inbound.Text)); err != nil {
		return nil, err
	}
	a.state.mu.Lock()
	a.state.turnCalls++
	a.state.prompt = turn.Inbound.Text
	a.state.mu.Unlock()
	return &component.TurnResult{
		Final: &coremodel.ThreadMessage{
			Kind:        coremodel.MessageKindAgent,
			ComponentID: a.componentID,
			ActorID:     "mockagent",
			ActorLabel:  "Mock Agent",
			Text:        "done",
		},
	}, nil
}

func newSQLiteStorage(t *testing.T) repository.Storage {
	t.Helper()
	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%s?mode=memory&cache=shared", name, modeluuid.New().String())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	storage := repository.NewGORM(db)
	if err := storage.AutoMigrate(context.Background()); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	return storage
}

func withTempCwd(t *testing.T, fn func(root string)) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir(%q) error = %v", root, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})
	fn(root)
}
