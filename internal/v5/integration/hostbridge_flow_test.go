package integration

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	v5broker "github.com/bartdeboer/ctgbot/internal/v5/broker"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	v5hostbridgeserver "github.com/bartdeboer/ctgbot/internal/v5/hostbridge/server"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
	v5system "github.com/bartdeboer/ctgbot/internal/v5/system"
	"github.com/bartdeboer/go-clir"
)

func init() {
	gob.Register(pingCommand{})
}

func skipIfHostbridgeListenerUnavailable(t *testing.T, bridge *v5hostbridgeserver.Bridge) {
	t.Helper()

	_, _, unregister, err := bridge.BindThread(modeluuid.New(), nil)
	if err == nil {
		if unregister != nil {
			unregister()
		}
		return
	}
	if isHostbridgeListenerUnavailable(err) {
		t.Skipf("hostbridge listener unavailable in this environment: %v", err)
	}
	t.Fatalf("hostbridge availability probe error = %v", err)
}

func isHostbridgeListenerUnavailable(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "bind: operation not permitted") ||
		strings.Contains(text, "listen tcp") && strings.Contains(text, "operation not permitted")
}

func TestV5HostbridgeFlow(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()

		storage := newSQLiteStorage(t)
		bridge := v5hostbridgeserver.NewBridge(root, storage, nil)
		t.Cleanup(func() {
			_ = bridge.Close()
		})
		skipIfHostbridgeListenerUnavailable(t, bridge)

		runtimeState := &runtimeState{}
		messengerState := &messengerState{
			event: component.InboundEvent{
				ExternalID: "msg-1",
				Payload: messenger.InboundPayload{
					ProviderType:      "mockmsg",
					ProviderChatID:    "chat-1",
					ProviderThreadID:  "provider-thread-1",
					ProviderMessageID: "msg-1",
					Actor:             actorWithRoles("", "bart"),
					Text:              messenger.TextMessage{Text: "hello"},
				},
			},
		}
		agentState := &agentState{}
		commandState := &commandState{}

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
			return &hostbridgeAgent{
				componentID: registration.ID,
				runtime:     runtime.Bind(registration, home, v5runtime.BindConfig{}),
				bridge:      bridge,
				state:       agentState,
			}, nil
		}); err != nil {
			t.Fatalf("register mockagent: %v", err)
		}
		if err := registry.Add("mockcmd", func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
			_, _, _, _, _ = ctx, runtime, home, storage, registration
			return &mockCommandComponent{state: commandState}, nil
		}); err != nil {
			t.Fatalf("register mockcmd: %v", err)
		}

		system := v5system.New(storage, map[string]v5system.Workspace{}, map[string]v5runtime.Factory{
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
		commandRegistration, err := system.EnsureComponent(ctx, "mockcmd", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockcmd) error = %v", err)
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
		if _, err := system.BindChatComponent(ctx, chat.ID, coremodel.ChatComponentRoleCommand, commandRegistration.Ref(), ""); err != nil {
			t.Fatalf("BindChatComponent(command) error = %v", err)
		}

		b := v5broker.New(storage, system, nil)
		if err := b.Run(ctx); err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if agentState.authCalls != 1 {
			t.Fatalf("auth calls = %d, want 1", agentState.authCalls)
		}
		if agentState.turnCalls != 1 {
			t.Fatalf("turn calls = %d, want 1", agentState.turnCalls)
		}
		if runtimeState.execCalls != 0 {
			t.Fatalf("runtime exec calls = %d, want 0", runtimeState.execCalls)
		}
		if commandState.calls != 1 {
			t.Fatalf("command calls = %d, want 1", commandState.calls)
		}
		if commandState.lastThreadID.IsNull() {
			t.Fatal("hostbridge command did not receive a thread id")
		}
		if commandState.lastChatID.IsNull() {
			t.Fatal("hostbridge command did not receive a chat id")
		}
		if commandState.lastSandboxID != commandState.lastThreadID {
			t.Fatalf("sandbox id = %s, want %s", commandState.lastSandboxID, commandState.lastThreadID)
		}
		if commandState.lastSource != commandengine.SourceHostbridge {
			t.Fatalf("source = %q, want %q", commandState.lastSource, commandengine.SourceHostbridge)
		}
		if commandState.lastActorID == "" {
			t.Fatal("expected hostbridge actor id")
		}
		if len(messengerState.relayPayloads) != 1 {
			t.Fatalf("relay payload count = %d, want 1", len(messengerState.relayPayloads))
		}
		if got := messengerState.relayPayloads[0].Text.Text; got != "pong" {
			t.Fatalf("relay text = %q, want pong", got)
		}
	})
}

func TestV5HostbridgeSendMediaFlow(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()

		storage := newSQLiteStorage(t)
		bridge := v5hostbridgeserver.NewBridge(root, storage, nil)
		t.Cleanup(func() {
			_ = bridge.Close()
		})
		skipIfHostbridgeListenerUnavailable(t, bridge)

		runtimeState := &runtimeState{}
		messengerState := &messengerState{
			event: component.InboundEvent{
				ExternalID: "msg-1",
				Payload: messenger.InboundPayload{
					ProviderType:      "mockmsg",
					ProviderChatID:    "chat-1",
					ProviderThreadID:  "provider-thread-1",
					ProviderMessageID: "msg-1",
					Actor:             actorWithRoles("", "bart"),
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
			return &hostbridgeMediaAgent{
				componentID: registration.ID,
				runtime:     runtime.Bind(registration, home, v5runtime.BindConfig{}),
				bridge:      bridge,
				state:       agentState,
			}, nil
		}); err != nil {
			t.Fatalf("register mockagent: %v", err)
		}

		system := v5system.New(storage, map[string]v5system.Workspace{}, map[string]v5runtime.Factory{
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

		b := v5broker.New(storage, system, nil)
		if err := b.Run(ctx); err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if agentState.authCalls != 1 {
			t.Fatalf("auth calls = %d, want 1", agentState.authCalls)
		}
		if agentState.turnCalls != 1 {
			t.Fatalf("turn calls = %d, want 1", agentState.turnCalls)
		}
		if runtimeState.execCalls != 0 {
			t.Fatalf("runtime exec calls = %d, want 0", runtimeState.execCalls)
		}
		if len(messengerState.relayPayloads) != 1 {
			t.Fatalf("relay payload count = %d, want 1", len(messengerState.relayPayloads))
		}
		payload := messengerState.relayPayloads[0]
		if got := payload.Text.Text; got != "artifact note" {
			t.Fatalf("relay text = %q, want artifact note", got)
		}
		if len(payload.Attachments) != 1 {
			t.Fatalf("relay attachment count = %d, want 1", len(payload.Attachments))
		}
		if got := payload.Attachments[0].Filename; got != "stdin.txt" {
			t.Fatalf("attachment filename = %q, want stdin.txt", got)
		}
		if got := string(payload.Attachments[0].Content); got != "hello from hostbridge" {
			t.Fatalf("attachment content = %q, want hello from hostbridge", got)
		}

		threads, err := storage.Threads().ListByChatID(ctx, chat.ID)
		if err != nil {
			t.Fatalf("ListByChatID() error = %v", err)
		}
		if len(threads) != 1 {
			t.Fatalf("thread count = %d, want 1", len(threads))
		}
		messages, err := storage.Messages().ListByThreadID(ctx, threads[0].ID)
		if err != nil {
			t.Fatalf("ListByThreadID() error = %v", err)
		}
		if len(messages) != 2 {
			t.Fatalf("stored messages = %d, want 2", len(messages))
		}
		if messages[1].Text != "artifact note" {
			t.Fatalf("stored outbound text = %q, want artifact note", messages[1].Text)
		}
		artifacts, err := storage.Artifacts().ListByMessageID(ctx, messages[1].ID)
		if err != nil {
			t.Fatalf("Artifacts().ListByMessageID() error = %v", err)
		}
		if len(artifacts) != 1 {
			t.Fatalf("stored artifacts = %d, want 1", len(artifacts))
		}
		if got := artifacts[0].Filename; got != "stdin.txt" {
			t.Fatalf("stored artifact filename = %q, want stdin.txt", got)
		}
		if got := string(artifacts[0].Content); got != "hello from hostbridge" {
			t.Fatalf("stored artifact content = %q, want hello from hostbridge", got)
		}
	})
}

func TestV5HostbridgeRunCommandFlow(t *testing.T) {
	if _, ok := hostbridgeserver.DefaultAllowedCommands()["pwd"]; !ok {
		t.Skip("default hostbridge command pwd is unavailable on this platform")
	}
	withTempCwd(t, func(root string) {
		ctx := context.Background()

		storage := newSQLiteStorage(t)
		bridge := v5hostbridgeserver.NewBridge(root, storage, nil)
		t.Cleanup(func() {
			_ = bridge.Close()
		})
		skipIfHostbridgeListenerUnavailable(t, bridge)

		runtimeState := &runtimeState{}
		messengerState := &messengerState{
			event: component.InboundEvent{
				ExternalID: "msg-run",
				Payload: messenger.InboundPayload{
					ProviderType:      "mockmsg",
					ProviderChatID:    "chat-1",
					ProviderThreadID:  "provider-thread-1",
					ProviderMessageID: "msg-run",
					Actor:             actorWithRoles("", "bart"),
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
			return &hostbridgeRunAgent{
				componentID: registration.ID,
				runtime:     runtime.Bind(registration, home, v5runtime.BindConfig{}),
				bridge:      bridge,
				command:     "pwd",
				state:       agentState,
			}, nil
		}); err != nil {
			t.Fatalf("register mockagent: %v", err)
		}

		system := v5system.New(storage, map[string]v5system.Workspace{}, map[string]v5runtime.Factory{
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

		b := v5broker.New(storage, system, nil)
		if err := b.Run(ctx); err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if agentState.authCalls != 1 {
			t.Fatalf("auth calls = %d, want 1", agentState.authCalls)
		}
		if agentState.turnCalls != 1 {
			t.Fatalf("turn calls = %d, want 1", agentState.turnCalls)
		}
		if runtimeState.execCalls != 0 {
			t.Fatalf("runtime exec calls = %d, want 0", runtimeState.execCalls)
		}
		if got, want := len(messengerState.relayPayloads), 1; got != want {
			t.Fatalf("relay payload count = %d, want %d", got, want)
		}
		if got := strings.TrimSpace(messengerState.relayPayloads[0].Text.Text); got == "" {
			t.Fatal("expected non-empty hostbridge run output")
		}
	})
}

func TestV5HostbridgeRunUsesWorkspaceAllowedCommands(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()

		storage := newSQLiteStorage(t)
		bridge := v5hostbridgeserver.NewBridge(root, storage, nil)
		t.Cleanup(func() {
			_ = bridge.Close()
		})
		skipIfHostbridgeListenerUnavailable(t, bridge)

		runtimeState := &runtimeState{}
		messengerState := &messengerState{
			event: component.InboundEvent{
				ExternalID: "msg-run-workspace",
				Payload: messenger.InboundPayload{
					ProviderType:      "mockmsg",
					ProviderChatID:    "chat-1",
					ProviderThreadID:  "provider-thread-1",
					ProviderMessageID: "msg-run-workspace",
					Actor:             actorWithRoles("", "bart"),
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
			return &hostbridgeRunAgent{
				componentID: registration.ID,
				runtime:     runtime.Bind(registration, home, v5runtime.BindConfig{}),
				bridge:      bridge,
				command:     "echo-workspace",
				state:       agentState,
			}, nil
		}); err != nil {
			t.Fatalf("register mockagent: %v", err)
		}

		system := v5system.New(storage, map[string]v5system.Workspace{
			"work": {
				Name: "work",
				Path: filepath.Join(root, "workspaces", "work"),
				HostbridgeAllowedCommands: map[string]hostbridgeserver.AllowedCommand{
					"echo-workspace": {
						Name: "/bin/echo",
						Args: []string{"workspace-ok"},
					},
				},
			},
		}, map[string]v5runtime.Factory{
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
			Label:     "team",
			Enabled:   true,
			Workspace: "work",
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

		if agentState.turnCalls != 1 {
			t.Fatalf("turn calls = %d, want 1", agentState.turnCalls)
		}
		if got, want := strings.TrimSpace(messengerState.relayPayloads[0].Text.Text), "workspace-ok"; got != want {
			t.Fatalf("relay text = %q, want %q", got, want)
		}
	})
}

type pingCommand struct{}

type commandState struct {
	calls         int
	lastThreadID  modeluuid.UUID
	lastChatID    modeluuid.UUID
	lastSandboxID modeluuid.UUID
	lastSource    commandengine.Source
	lastActorID   string
}

type mockCommandComponent struct {
	state *commandState
}

func (c *mockCommandComponent) Type() string {
	return "mockcmd"
}

func (c *mockCommandComponent) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{{
		Pattern: "mockcmd ping",
		Help:    "Reply with pong",
		Build: func(req *clir.Request) (any, error) {
			_ = req
			return pingCommand{}, nil
		},
		Sources: []commandengine.Source{commandengine.SourceHostbridge},
		Policy:  simplerbac.Public(),
	}}
}

func (c *mockCommandComponent) RegisterCommandHandlers(registry *commandengine.Registry) error {
	return commandengine.Register[pingCommand](registry, func(ctx context.Context, req commandengine.Request, cmd pingCommand) (commandengine.Result, error) {
		_, _ = ctx, cmd
		c.state.calls++
		c.state.lastThreadID = req.Context.ThreadID
		c.state.lastChatID = req.Context.ChatID
		c.state.lastSandboxID = req.Context.SandboxID
		c.state.lastSource = req.Context.Source
		c.state.lastActorID = req.Context.Actor.ID
		return commandengine.Result{Text: "pong"}, nil
	})
}

type hostbridgeAgent struct {
	componentID modeluuid.UUID
	runtime     v5runtime.Runtime
	bridge      *v5hostbridgeserver.Bridge
	state       *agentState
}

type hostbridgeMediaAgent struct {
	componentID modeluuid.UUID
	runtime     v5runtime.Runtime
	bridge      *v5hostbridgeserver.Bridge
	state       *agentState
}

type hostbridgeRunAgent struct {
	componentID modeluuid.UUID
	runtime     v5runtime.Runtime
	bridge      *v5hostbridgeserver.Bridge
	command     string
	state       *agentState
}

func (a *hostbridgeMediaAgent) Type() string {
	return "mockagent"
}

func (a *hostbridgeMediaAgent) Auth(ctx context.Context, callbackPort int, callbackTimeout time.Duration, stdout io.Writer, stderr io.Writer) error {
	_, _, _, _, _ = ctx, callbackPort, callbackTimeout, stdout, stderr
	home := a.runtime.ComponentHome()
	if err := os.MkdirAll(home.Path, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(home.Path, "auth.json"), []byte(`{"ok":true}`), 0o600); err != nil {
		return err
	}
	a.state.authCalls++
	return nil
}

func (a *hostbridgeMediaAgent) HandleTurn(ctx context.Context, turn component.Turn) (*component.TurnResult, error) {
	if _, err := os.Stat(filepath.Join(a.runtime.ComponentHome().Path, "auth.json")); err != nil {
		return nil, fmt.Errorf("missing auth.json: %w", err)
	}

	_, err := a.bridge.DoCommand(ctx, turn.Thread.ID, turn.Runtime.Commands(), commandengine.Request{
		Context: commandengine.Context{
			SandboxID: turn.Thread.ID,
		},
		Command: schemacommands.SendMedia{
			Filename:    "stdin.txt",
			Caption:     "artifact note",
			ContentType: "text/plain",
			Syntax:      "markdown",
			Content:     []byte("hello from hostbridge"),
		},
		CanonicalPattern: "sendstdin",
		Route:            "sendstdin",
	})
	if err != nil {
		return nil, err
	}

	a.state.turnCalls++
	a.state.prompt = turn.Inbound.Text
	return nil, nil
}

func (a *hostbridgeAgent) Type() string {
	return "mockagent"
}

func (a *hostbridgeRunAgent) Type() string {
	return "mockagent"
}

func (a *hostbridgeAgent) Auth(ctx context.Context, callbackPort int, callbackTimeout time.Duration, stdout io.Writer, stderr io.Writer) error {
	_, _, _, _, _ = ctx, callbackPort, callbackTimeout, stdout, stderr
	home := a.runtime.ComponentHome()
	if err := os.MkdirAll(home.Path, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(home.Path, "auth.json"), []byte(`{"ok":true}`), 0o600); err != nil {
		return err
	}
	a.state.authCalls++
	return nil
}

func (a *hostbridgeRunAgent) Auth(ctx context.Context, callbackPort int, callbackTimeout time.Duration, stdout io.Writer, stderr io.Writer) error {
	_, _, _, _, _ = ctx, callbackPort, callbackTimeout, stdout, stderr
	home := a.runtime.ComponentHome()
	if err := os.MkdirAll(home.Path, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(home.Path, "auth.json"), []byte(`{"ok":true}`), 0o600); err != nil {
		return err
	}
	a.state.authCalls++
	return nil
}

func (a *hostbridgeAgent) HandleTurn(ctx context.Context, turn component.Turn) (*component.TurnResult, error) {
	if _, err := os.Stat(filepath.Join(a.runtime.ComponentHome().Path, "auth.json")); err != nil {
		return nil, fmt.Errorf("missing auth.json: %w", err)
	}

	result, err := a.bridge.DoCommand(ctx, turn.Thread.ID, turn.Runtime.Commands(), commandengine.Request{
		Context: commandengine.Context{
			SandboxID: turn.Thread.ID,
		},
		Command:          pingCommand{},
		CanonicalPattern: "mockcmd ping",
		Route:            "mockcmd ping",
	})
	if err != nil {
		return nil, err
	}

	a.state.turnCalls++
	a.state.prompt = turn.Inbound.Text
	return &component.TurnResult{
		Final: &coremodel.ThreadMessage{
			Kind:        coremodel.MessageKindAgent,
			ComponentID: a.componentID,
			ActorID:     "mockagent",
			ActorLabel:  "Mock Agent",
			Text:        result.Text,
		},
	}, nil
}

func (a *hostbridgeRunAgent) HandleTurn(ctx context.Context, turn component.Turn) (*component.TurnResult, error) {
	if _, err := os.Stat(filepath.Join(a.runtime.ComponentHome().Path, "auth.json")); err != nil {
		return nil, fmt.Errorf("missing auth.json: %w", err)
	}
	commandName := strings.TrimSpace(a.command)
	if commandName == "" {
		commandName = "pwd"
	}

	result, err := a.bridge.DoCommand(ctx, turn.Thread.ID, turn.Runtime.Commands(), commandengine.Request{
		Context: commandengine.Context{
			SandboxID: turn.Thread.ID,
		},
		Command: schemacommands.RunCommand{
			Command: commandName,
		},
		CanonicalPattern: "run <name>",
		Route:            "run " + commandName,
	})
	if err != nil {
		return nil, err
	}

	a.state.turnCalls++
	a.state.prompt = turn.Inbound.Text
	return &component.TurnResult{
		Final: &coremodel.ThreadMessage{
			Kind:        coremodel.MessageKindAgent,
			ComponentID: a.componentID,
			ActorID:     "mockagent",
			ActorLabel:  "Mock Agent",
			Text:        strings.TrimSpace(result.Text),
		},
	}, nil
}
