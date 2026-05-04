package integration

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	v5broker "github.com/bartdeboer/ctgbot/internal/v5/broker"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
	v5system "github.com/bartdeboer/ctgbot/internal/v5/system"
	"github.com/bartdeboer/go-clistate"
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
				profile:        profile,
				rootDir:        root,
				componentsRoot: filepath.Join(root, ".ctgbot", "components"),
				state:          runtimeState,
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

		if err := system.AuthComponent(ctx, "mockagent", "", 0, 0, io.Discard, io.Discard); err != nil {
			t.Fatalf("AuthComponent() error = %v", err)
		}

		authPath := filepath.Join(root, ".ctgbot", "components", "mockagent", "mockagent", "auth.json")
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
