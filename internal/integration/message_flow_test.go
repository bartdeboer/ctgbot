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

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	systempkg "github.com/bartdeboer/ctgbot/internal/system"
)

func TestMockComponentsEndToEnd(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()

		storage := newSQLiteStorage(t)
		runtimeState := &runtimeState{}
		messengerState := &messengerState{
			event: component.InboundEvent{
				ExternalID: "msg-1",
				Payload: message.InboundPayload{
					ProviderType:      "mockmsg",
					ProviderChannelID: "chat-1",
					ProviderThreadID:  "provider-thread-1",
					ProviderMessageID: "msg-1",
					Actor:             actorWithRoles("", "bart"),
					Text:              message.TextMessage{Text: "hello"},
				},
			},
		}
		agentState := &agentState{}

		registry := component.NewRegistry()
		if err := registry.Add("mockmsg", func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
			_, _, _, _, _ = ctx, runtime, home, storage, registration
			return &mockMessenger{
				componentID: registration.ID,
				state:       messengerState,
			}, nil
		}); err != nil {
			t.Fatalf("register mockmsg: %v", err)
		}
		if err := registry.Add("mockagent", func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
			_, _, _ = ctx, home, storage
			return &mockAgent{
				componentID: registration.ID,
				runtime:     runtime.(runtimepkg.ThreadRuntimeFactory).Bind(registration, home, runtimepkg.BindConfig{}),
				state:       agentState,
			}, nil
		}); err != nil {
			t.Fatalf("register mockagent: %v", err)
		}

		system := systempkg.New(storage, map[string]systempkg.Workspace{}, map[string]runtimepkg.Factory{
			"local": fakeRuntimeFactory{
				runtimeKind:    "local",
				rootDir:        root,
				componentsRoot: filepath.Join(root, ".ctgbot", "components"),
				state:          runtimeState,
			},
		}, registry)
		system.StateRoot = filepath.Join(root, ".ctgbot")

		messengerRegistration, err := system.EnsureComponent(ctx, "mockmsg", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockmsg) error = %v", err)
		}
		agentRegistration, err := system.EnsureComponent(ctx, "mockagent", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockagent) error = %v", err)
		}

		if err := system.AuthComponent(ctx, "mockagent", "", "", 0, 0, io.Discard, io.Discard); err != nil {
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

		b := newTestBroker(storage, system, nil)
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
		if runtimeState.stopCalls != 0 {
			t.Fatalf("stop calls = %d, want 0 for generic mock agent", runtimeState.stopCalls)
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
		if payload.ProviderChannelID != "chat-1" {
			t.Fatalf("relay provider channel id = %q, want chat-1", payload.ProviderChannelID)
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

func TestInboundAttachmentsMaterializeIntoWorkspaceInboxAndInjectPrompt(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()

		storage := newSQLiteStorage(t)
		runtimeState := &runtimeState{}
		messengerState := &messengerState{
			event: component.InboundEvent{
				ExternalID: "msg-attach",
				Payload: message.InboundPayload{
					ProviderType:      "mockmsg",
					ProviderChannelID: "chat-1",
					ProviderThreadID:  "provider-thread-1",
					ProviderMessageID: "msg-attach",
					Actor:             actorWithRoles("", "bart"),
					Text:              message.TextMessage{Text: "review this file"},
					Attachments: []message.Media{{
						Filename:    "note.txt",
						ContentType: "text/plain",
						Content:     []byte("hello attachment"),
					}},
				},
			},
		}
		agentState := &agentState{}

		registry := component.NewRegistry()
		if err := registry.Add("mockmsg", func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
			_, _, _, _, _ = ctx, runtime, home, storage, registration
			return &mockMessenger{
				componentID: registration.ID,
				state:       messengerState,
			}, nil
		}); err != nil {
			t.Fatalf("register mockmsg: %v", err)
		}
		if err := registry.Add("mockagent", func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
			_, _, _ = ctx, home, storage
			return &mockAgent{
				componentID: registration.ID,
				runtime:     runtime.(runtimepkg.ThreadRuntimeFactory).Bind(registration, home, runtimepkg.BindConfig{}),
				state:       agentState,
			}, nil
		}); err != nil {
			t.Fatalf("register mockagent: %v", err)
		}

		system := systempkg.New(storage, map[string]systempkg.Workspace{}, map[string]runtimepkg.Factory{
			"local": fakeRuntimeFactory{
				runtimeKind:    "local",
				rootDir:        root,
				componentsRoot: filepath.Join(root, ".ctgbot", "components"),
				state:          runtimeState,
			},
		}, registry)
		system.StateRoot = filepath.Join(root, ".ctgbot")

		messengerRegistration, err := system.EnsureComponent(ctx, "mockmsg", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockmsg) error = %v", err)
		}
		agentRegistration, err := system.EnsureComponent(ctx, "mockagent", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockagent) error = %v", err)
		}
		if err := system.AuthComponent(ctx, "mockagent", "", "", 0, 0, io.Discard, io.Discard); err != nil {
			t.Fatalf("AuthComponent() error = %v", err)
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

		b := newTestBroker(storage, system, nil)
		if err := b.Run(ctx); err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		wantPath := filepath.ToSlash(filepath.Join(root, ".ctgbot", "chats", chat.ID.String(), "workspace", "inbox", "note.txt"))
		if !strings.Contains(agentState.prompt, "Files made available:") {
			t.Fatalf("prompt missing attachment prelude:\n%s", agentState.prompt)
		}
		if !strings.Contains(agentState.prompt, wantPath) {
			t.Fatalf("prompt missing runtime file path %q:\n%s", wantPath, agentState.prompt)
		}
		if !strings.Contains(agentState.prompt, "review this file") {
			t.Fatalf("prompt missing original message:\n%s", agentState.prompt)
		}

		threads, err := storage.Threads().ListByChatID(ctx, chat.ID)
		if err != nil {
			t.Fatalf("Threads().ListByChatID() error = %v", err)
		}
		if len(threads) != 1 {
			t.Fatalf("thread count = %d, want 1", len(threads))
		}
		inboxPath := filepath.Join(root, ".ctgbot", "chats", threads[0].ChatID.String(), "workspace", "inbox", "note.txt")
		content, err := os.ReadFile(inboxPath)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", inboxPath, err)
		}
		if got, want := string(content), "hello attachment"; got != want {
			t.Fatalf("inbox content = %q, want %q", got, want)
		}
		messages, err := storage.Messages().ListByThreadID(ctx, threads[0].ID)
		if err != nil {
			t.Fatalf("Messages().ListByThreadID() error = %v", err)
		}
		if len(messages) != 2 {
			t.Fatalf("stored messages = %d, want 2", len(messages))
		}
		artifacts, err := storage.Artifacts().ListByMessageID(ctx, messages[0].ID)
		if err != nil {
			t.Fatalf("Artifacts().ListByMessageID() error = %v", err)
		}
		if len(artifacts) != 1 {
			t.Fatalf("artifacts = %d, want 1", len(artifacts))
		}
		if got, want := artifacts[0].Filename, "note.txt"; got != want {
			t.Fatalf("artifact filename = %q, want %q", got, want)
		}
	})
}

func TestAttachmentOnlyInboundReturnsUploadSavedMessage(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()

		storage := newSQLiteStorage(t)
		runtimeState := &runtimeState{}
		messengerState := &messengerState{
			event: component.InboundEvent{
				ExternalID: "msg-upload-only",
				Payload: message.InboundPayload{
					ProviderType:      "mockmsg",
					ProviderChannelID: "chat-1",
					ProviderThreadID:  "provider-thread-1",
					ProviderMessageID: "msg-upload-only",
					Actor:             actorWithRoles("", "bart"),
					Attachments: []message.Media{{
						Filename:    "note.txt",
						ContentType: "text/plain",
						Content:     []byte("hello attachment"),
					}},
				},
			},
		}
		agentState := &agentState{}

		registry := component.NewRegistry()
		if err := registry.Add("mockmsg", func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
			_, _, _, _, _ = ctx, runtime, home, storage, registration
			return &mockMessenger{
				componentID: registration.ID,
				state:       messengerState,
			}, nil
		}); err != nil {
			t.Fatalf("register mockmsg: %v", err)
		}
		if err := registry.Add("mockagent", func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
			_, _, _ = ctx, home, storage
			return &mockAgent{
				componentID: registration.ID,
				runtime:     runtime.(runtimepkg.ThreadRuntimeFactory).Bind(registration, home, runtimepkg.BindConfig{}),
				state:       agentState,
			}, nil
		}); err != nil {
			t.Fatalf("register mockagent: %v", err)
		}

		system := systempkg.New(storage, map[string]systempkg.Workspace{}, map[string]runtimepkg.Factory{
			"local": fakeRuntimeFactory{
				runtimeKind:    "local",
				rootDir:        root,
				componentsRoot: filepath.Join(root, ".ctgbot", "components"),
				state:          runtimeState,
			},
		}, registry)
		system.StateRoot = filepath.Join(root, ".ctgbot")

		messengerRegistration, err := system.EnsureComponent(ctx, "mockmsg", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockmsg) error = %v", err)
		}
		agentRegistration, err := system.EnsureComponent(ctx, "mockagent", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockagent) error = %v", err)
		}

		chat := &coremodel.Chat{Label: "team", Enabled: true}
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

		b := newTestBroker(storage, system, nil)
		if err := b.Run(ctx); err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if agentState.turnCalls != 0 {
			t.Fatalf("agent turn calls = %d, want 0", agentState.turnCalls)
		}
		if len(messengerState.relayPayloads) != 1 {
			t.Fatalf("relay payload count = %d, want 1", len(messengerState.relayPayloads))
		}
		wantPath := filepath.ToSlash(filepath.Join(root, ".ctgbot", "chats", chat.ID.String(), "workspace", "inbox", "note.txt"))
		if got, want := messengerState.relayPayloads[0].Text.Text, "upload saved: "+wantPath; got != want {
			t.Fatalf("relay text = %q, want %q", got, want)
		}
	})
}

func TestConversationErrorIsReportedToChatAndDoesNotStopSource(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()

		storage := newSQLiteStorage(t)
		runtimeState := &runtimeState{}
		sourceState := &multiEventSourceState{
			events: []component.InboundEvent{
				{
					ExternalID: "msg-1",
					Payload: message.InboundPayload{
						ProviderType:      "mockmsg",
						ProviderChannelID: "chat-1",
						ProviderThreadID:  "provider-thread-1",
						ProviderMessageID: "msg-1",
						Actor:             actorWithRoles("", "bart"),
						Text:              message.TextMessage{Text: "boom"},
					},
				},
				{
					ExternalID: "msg-2",
					Payload: message.InboundPayload{
						ProviderType:      "mockmsg",
						ProviderChannelID: "chat-1",
						ProviderThreadID:  "provider-thread-1",
						ProviderMessageID: "msg-2",
						Actor:             actorWithRoles("", "bart"),
						Text:              message.TextMessage{Text: "hello"},
					},
				},
			},
		}
		relayState := &relayState{}
		agentState := &agentState{}

		registry := component.NewRegistry()
		if err := registry.Add("mockmsg", func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
			_, _, _, _, _ = ctx, runtime, home, storage, registration
			return &multiEventSource{
				componentID: registration.ID,
				state:       sourceState,
			}, nil
		}); err != nil {
			t.Fatalf("register mockmsg: %v", err)
		}
		if err := registry.Add("mockrelay", func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
			_, _, _, _, _ = ctx, runtime, home, storage, registration
			return &mockRelay{state: relayState}, nil
		}); err != nil {
			t.Fatalf("register mockrelay: %v", err)
		}
		if err := registry.Add("failingagent", func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
			_, _, _ = ctx, home, storage
			return &failingAgent{
				componentID: registration.ID,
				runtime:     runtime.(runtimepkg.ThreadRuntimeFactory).Bind(registration, home, runtimepkg.BindConfig{}),
				state:       agentState,
			}, nil
		}); err != nil {
			t.Fatalf("register failingagent: %v", err)
		}

		system := systempkg.New(storage, map[string]systempkg.Workspace{}, map[string]runtimepkg.Factory{
			"local": fakeRuntimeFactory{
				runtimeKind:    "local",
				rootDir:        root,
				componentsRoot: filepath.Join(root, ".ctgbot", "components"),
				state:          runtimeState,
			},
		}, registry)
		system.StateRoot = filepath.Join(root, ".ctgbot")

		sourceRegistration, err := system.EnsureComponent(ctx, "mockmsg", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockmsg) error = %v", err)
		}
		relayRegistration, err := system.EnsureComponent(ctx, "mockrelay", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockrelay) error = %v", err)
		}
		agentRegistration, err := system.EnsureComponent(ctx, "failingagent", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(failingagent) error = %v", err)
		}

		chat := &coremodel.Chat{Label: "team", Enabled: true}
		if err := storage.Chats().Save(ctx, chat); err != nil {
			t.Fatalf("Chats().Save() error = %v", err)
		}
		if _, err := system.BindChatComponent(ctx, chat.ID, coremodel.ChatComponentRoleSource, sourceRegistration.Ref(), "chat-1"); err != nil {
			t.Fatalf("BindChatComponent(source) error = %v", err)
		}
		if _, err := system.BindChatComponent(ctx, chat.ID, coremodel.ChatComponentRoleRelay, relayRegistration.Ref(), "chat-1"); err != nil {
			t.Fatalf("BindChatComponent(relay) error = %v", err)
		}
		if _, err := system.BindChatComponent(ctx, chat.ID, coremodel.ChatComponentRoleAgent, agentRegistration.Ref(), ""); err != nil {
			t.Fatalf("BindChatComponent(agent) error = %v", err)
		}

		b := newTestBroker(storage, system, nil)
		if err := b.Run(ctx); err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if sourceState.runCalls != 1 {
			t.Fatalf("source run calls = %d, want 1", sourceState.runCalls)
		}
		if agentState.turnCalls != 2 {
			t.Fatalf("agent turn calls = %d, want 2", agentState.turnCalls)
		}
		if len(relayState.payloads) != 2 {
			t.Fatalf("relay payload count = %d, want 2", len(relayState.payloads))
		}
		if got := relayState.payloads[0].Text.Text; !strings.HasPrefix(got, "conversation error:") {
			t.Fatalf("first relay text = %q, want conversation error", got)
		}
		if got, want := relayState.payloads[1].Text.Text, "done"; got != want {
			t.Fatalf("second relay text = %q, want %q", got, want)
		}
	})
}

type mockAgent struct {
	componentID modeluuid.UUID
	runtime     runtimepkg.ThreadRuntime
	state       *agentState
}

type failingAgent struct {
	componentID modeluuid.UUID
	runtime     runtimepkg.ThreadRuntime
	state       *agentState
}

func (a *failingAgent) Type() string {
	return "failingagent"
}

func (a *failingAgent) HandleTurn(ctx context.Context, turn component.Turn) (*component.TurnResult, error) {
	_ = ctx
	a.state.mu.Lock()
	a.state.turnCalls++
	a.state.prompt = turn.Inbound.Text
	a.state.mu.Unlock()
	if strings.TrimSpace(turn.Inbound.Text) == "boom" {
		return nil, fmt.Errorf("boom")
	}
	return &component.TurnResult{
		Final: &coremodel.ThreadMessage{
			Kind:        coremodel.MessageKindAgent,
			ComponentID: a.componentID,
			ActorID:     "failingagent",
			ActorLabel:  "Failing Agent",
			Text:        "done",
		},
	}, nil
}

func (a *mockAgent) Type() string {
	return "mockagent"
}

func (a *mockAgent) Auth(ctx context.Context, callbackPort int, callbackTimeout time.Duration, stdout io.Writer, stderr io.Writer) error {
	_, _, _, _, _ = ctx, callbackPort, callbackTimeout, stdout, stderr
	home := a.runtime.ComponentHome()
	if err := os.MkdirAll(home.Path, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(home.Path, "auth.json"), []byte(`{"ok":true}`), 0o600); err != nil {
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
	if homeFromRuntime.Path != homeFromTurn.Path {
		return nil, fmt.Errorf("component home mismatch: %s != %s", homeFromRuntime.Path, homeFromTurn.Path)
	}
	if _, err := os.Stat(filepath.Join(homeFromRuntime.Path, "auth.json")); err != nil {
		return nil, fmt.Errorf("missing auth.json: %w", err)
	}
	if err := a.runtime.Exec(ctx, turn.Runtime.WorkspacePath(), turn.Thread.ID, turn.Runtime.Commands(), io.Discard, io.Discard, "mock-agent", "reply", strings.TrimSpace(turn.Inbound.Text)); err != nil {
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
