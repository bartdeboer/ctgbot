package integration

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	v5broker "github.com/bartdeboer/ctgbot/internal/v5/broker"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	v5process "github.com/bartdeboer/ctgbot/internal/v5/component/process"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
	v5system "github.com/bartdeboer/ctgbot/internal/v5/system"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func TestV5MessageCommandRunsAndSkipsAgent(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		system, storage, messengerState, agentState, runtimeState := newMessageCommandTestSystem(
			t,
			root,
			component.InboundEvent{
				ExternalID: "msg-1",
				Payload: messenger.InboundPayload{
					ProviderType:      "mockmsg",
					ProviderChatID:    "chat-1",
					ProviderThreadID:  "provider-thread-1",
					ProviderMessageID: "msg-1",
					Actor:             actorWithRoles("", "bart"),
					Text:              messenger.TextMessage{Text: "/tools ping"},
				},
			},
			func(registry *component.Registry) error {
				return registry.Add("tools", func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
					_, _, _, _, _ = ctx, registration, runtime, home, storage
					return &mockMessageCommandComponent{}, nil
				})
			},
		)

		messengerRegistration, err := system.EnsureComponent(ctx, "mockmsg", "test")
		if err != nil {
			t.Fatalf("EnsureComponent(mockmsg) error = %v", err)
		}
		agentRegistration, err := system.EnsureComponent(ctx, "mockagent", "test")
		if err != nil {
			t.Fatalf("EnsureComponent(mockagent) error = %v", err)
		}
		commandRegistration, err := system.EnsureComponent(ctx, "tools", "test")
		if err != nil {
			t.Fatalf("EnsureComponent(tools) error = %v", err)
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
		if _, err := system.BindChatComponent(ctx, chat.ID, coremodel.ChatComponentRoleCommand, commandRegistration.Ref(), ""); err != nil {
			t.Fatalf("BindChatComponent(command) error = %v", err)
		}

		b := v5broker.New(storage, system, nil)
		if err := b.Run(ctx); err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if messengerState.runCalls != 1 {
			t.Fatalf("run calls = %d, want 1", messengerState.runCalls)
		}
		if agentState.turnCalls != 0 {
			t.Fatalf("agent turn calls = %d, want 0", agentState.turnCalls)
		}
		if runtimeState.execCalls != 0 {
			t.Fatalf("runtime exec calls = %d, want 0", runtimeState.execCalls)
		}
		if got, want := len(messengerState.relayPayloads), 1; got != want {
			t.Fatalf("relay payload count = %d, want %d", got, want)
		}
		if got, want := messengerState.relayPayloads[0].Text.Text, "pong"; got != want {
			t.Fatalf("relay text = %q, want %q", got, want)
		}
		threads, err := storage.Threads().ListByChatID(ctx, chat.ID)
		if err != nil {
			t.Fatalf("Threads().ListByChatID() error = %v", err)
		}
		if got, want := len(threads), 1; got != want {
			t.Fatalf("thread count = %d, want %d", got, want)
		}
		messages, err := storage.Messages().ListByThreadID(ctx, threads[0].ID)
		if err != nil {
			t.Fatalf("Messages().ListByThreadID() error = %v", err)
		}
		if got, want := len(messages), 1; got != want {
			t.Fatalf("stored messages = %d, want %d", got, want)
		}
		if got, want := messages[0].Text, "pong"; got != want {
			t.Fatalf("stored message text = %q, want %q", got, want)
		}
	})
}

func TestV5UnknownMessageCommandReturnsErrorAndSkipsAgent(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		system, storage, messengerState, agentState, runtimeState := newMessageCommandTestSystem(
			t,
			root,
			component.InboundEvent{
				ExternalID: "msg-1",
				Payload: messenger.InboundPayload{
					ProviderType:      "mockmsg",
					ProviderChatID:    "chat-1",
					ProviderThreadID:  "provider-thread-1",
					ProviderMessageID: "msg-1",
					Actor:             actorWithRoles("", "bart"),
					Text:              messenger.TextMessage{Text: "/tools nope"},
				},
			},
			func(registry *component.Registry) error {
				return registry.Add("tools", func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
					_, _, _, _, _ = ctx, registration, runtime, home, storage
					return &mockMessageCommandComponent{}, nil
				})
			},
		)

		messengerRegistration, err := system.EnsureComponent(ctx, "mockmsg", "test")
		if err != nil {
			t.Fatalf("EnsureComponent(mockmsg) error = %v", err)
		}
		agentRegistration, err := system.EnsureComponent(ctx, "mockagent", "test")
		if err != nil {
			t.Fatalf("EnsureComponent(mockagent) error = %v", err)
		}
		commandRegistration, err := system.EnsureComponent(ctx, "tools", "test")
		if err != nil {
			t.Fatalf("EnsureComponent(tools) error = %v", err)
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
		if _, err := system.BindChatComponent(ctx, chat.ID, coremodel.ChatComponentRoleCommand, commandRegistration.Ref(), ""); err != nil {
			t.Fatalf("BindChatComponent(command) error = %v", err)
		}

		b := v5broker.New(storage, system, nil)
		if err := b.Run(ctx); err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if agentState.turnCalls != 0 {
			t.Fatalf("agent turn calls = %d, want 0", agentState.turnCalls)
		}
		if runtimeState.execCalls != 0 {
			t.Fatalf("runtime exec calls = %d, want 0", runtimeState.execCalls)
		}
		if got, want := len(messengerState.relayPayloads), 1; got != want {
			t.Fatalf("relay payload count = %d, want %d", got, want)
		}
		if got := messengerState.relayPayloads[0].Text.Text; !strings.HasPrefix(got, "command error:") {
			t.Fatalf("relay text = %q, want command error", got)
		}
		threads, err := storage.Threads().ListByChatID(ctx, chat.ID)
		if err != nil {
			t.Fatalf("Threads().ListByChatID() error = %v", err)
		}
		if got, want := len(threads), 1; got != want {
			t.Fatalf("thread count = %d, want %d", got, want)
		}
		messages, err := storage.Messages().ListByThreadID(ctx, threads[0].ID)
		if err != nil {
			t.Fatalf("Messages().ListByThreadID() error = %v", err)
		}
		if got, want := len(messages), 1; got != want {
			t.Fatalf("stored messages = %d, want %d", got, want)
		}
	})
}

func TestV5ProcessQuitMessageAliasesAreIntercepted(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		for _, text := range []string{"/quit", "/process quit"} {
			system, storage, messengerState, agentState, runtimeState := newMessageCommandTestSystem(
				t,
				root,
				component.InboundEvent{
					ExternalID: "msg-" + text,
					Payload: messenger.InboundPayload{
						ProviderType:      "mockmsg",
						ProviderChatID:    "chat-1",
						ProviderThreadID:  "provider-thread-1",
						ProviderMessageID: "msg-" + text,
						Actor:             actorWithRoles("", "bart"),
						Text:              messenger.TextMessage{Text: text},
					},
				},
				func(registry *component.Registry) error {
					return registry.Add(v5process.Type, func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
						_, _, _, _, _ = ctx, registration, runtime, home, storage
						return v5process.New(&noopProcessActions{}), nil
					})
				},
			)

			messengerRegistration, err := system.EnsureComponent(ctx, "mockmsg", "test")
			if err != nil {
				t.Fatalf("EnsureComponent(mockmsg) error = %v", err)
			}
			agentRegistration, err := system.EnsureComponent(ctx, "mockagent", "test")
			if err != nil {
				t.Fatalf("EnsureComponent(mockagent) error = %v", err)
			}
			commandRegistration, err := system.EnsureComponent(ctx, v5process.Type, "test")
			if err != nil {
				t.Fatalf("EnsureComponent(process) error = %v", err)
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
			if _, err := system.BindChatComponent(ctx, chat.ID, coremodel.ChatComponentRoleCommand, commandRegistration.Ref(), ""); err != nil {
				t.Fatalf("BindChatComponent(command) error = %v", err)
			}

			b := v5broker.New(storage, system, nil)
			if err := b.Run(ctx); err != nil {
				t.Fatalf("Run(%q) error = %v", text, err)
			}

			if agentState.turnCalls != 0 {
				t.Fatalf("agent turn calls for %q = %d, want 0", text, agentState.turnCalls)
			}
			if runtimeState.execCalls != 0 {
				t.Fatalf("runtime exec calls for %q = %d, want 0", text, runtimeState.execCalls)
			}
			if got, want := len(messengerState.relayPayloads), 1; got != want {
				t.Fatalf("relay payload count for %q = %d, want %d", text, got, want)
			}
			if got := messengerState.relayPayloads[0].Text.Text; !strings.HasPrefix(got, "command error:") {
				t.Fatalf("relay text for %q = %q, want command error", text, got)
			}
			threads, err := storage.Threads().ListByChatID(ctx, chat.ID)
			if err != nil {
				t.Fatalf("Threads().ListByChatID(%q) error = %v", text, err)
			}
			if got, want := len(threads), 1; got != want {
				t.Fatalf("thread count for %q = %d, want %d", text, got, want)
			}
			messages, err := storage.Messages().ListByThreadID(ctx, threads[0].ID)
			if err != nil {
				t.Fatalf("Messages().ListByThreadID(%q) error = %v", text, err)
			}
			if got, want := len(messages), 1; got != want {
				t.Fatalf("stored messages for %q = %d, want %d", text, got, want)
			}
		}
	})
}

func TestV5ProcessQuitMessageAliasesAllowOperators(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		for _, text := range []string{"/quit", "/process quit"} {
			system, storage, messengerState, agentState, runtimeState := newMessageCommandTestSystem(
				t,
				root,
				component.InboundEvent{
					ExternalID: "operator-" + text,
					Payload: messenger.InboundPayload{
						ProviderType:      "mockmsg",
						ProviderChatID:    "chat-1",
						ProviderThreadID:  "provider-thread-1",
						ProviderMessageID: "operator-" + text,
						Actor:             actorWithRoles("13145044", "bart", simplerbac.RoleUser, simplerbac.RoleRoot),
						Text:              messenger.TextMessage{Text: text},
					},
				},
				func(registry *component.Registry) error {
					return registry.Add(v5process.Type, func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
						_, _, _, _, _ = ctx, registration, runtime, home, storage
						return v5process.New(&noopProcessActions{}), nil
					})
				},
			)

			messengerRegistration, err := system.EnsureComponent(ctx, "mockmsg", "test")
			if err != nil {
				t.Fatalf("EnsureComponent(mockmsg) error = %v", err)
			}
			agentRegistration, err := system.EnsureComponent(ctx, "mockagent", "test")
			if err != nil {
				t.Fatalf("EnsureComponent(mockagent) error = %v", err)
			}
			commandRegistration, err := system.EnsureComponent(ctx, v5process.Type, "test")
			if err != nil {
				t.Fatalf("EnsureComponent(process) error = %v", err)
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
			if _, err := system.BindChatComponent(ctx, chat.ID, coremodel.ChatComponentRoleCommand, commandRegistration.Ref(), ""); err != nil {
				t.Fatalf("BindChatComponent(command) error = %v", err)
			}

			b := v5broker.New(storage, system, nil)
			if err := b.Run(ctx); err != nil {
				t.Fatalf("Run(%q) error = %v", text, err)
			}

			if agentState.turnCalls != 0 {
				t.Fatalf("agent turn calls for %q = %d, want 0", text, agentState.turnCalls)
			}
			if runtimeState.execCalls != 0 {
				t.Fatalf("runtime exec calls for %q = %d, want 0", text, runtimeState.execCalls)
			}
			if got, want := len(messengerState.relayPayloads), 1; got != want {
				t.Fatalf("relay payload count for %q = %d, want %d", text, got, want)
			}
			if got, want := messengerState.relayPayloads[0].Text.Text, "quit requested"; got != want {
				t.Fatalf("relay text for %q = %q, want %q", text, got, want)
			}
			threads, err := storage.Threads().ListByChatID(ctx, chat.ID)
			if err != nil {
				t.Fatalf("Threads().ListByChatID(%q) error = %v", text, err)
			}
			if got, want := len(threads), 1; got != want {
				t.Fatalf("thread count for %q = %d, want %d", text, got, want)
			}
			messages, err := storage.Messages().ListByThreadID(ctx, threads[0].ID)
			if err != nil {
				t.Fatalf("Messages().ListByThreadID(%q) error = %v", text, err)
			}
			if got, want := len(messages), 1; got != want {
				t.Fatalf("stored messages for %q = %d, want %d", text, got, want)
			}
			if got, want := messages[0].Text, "quit requested"; got != want {
				t.Fatalf("stored message text for %q = %q, want %q", text, got, want)
			}
		}
	})
}

func newMessageCommandTestSystem(
	t *testing.T,
	root string,
	event component.InboundEvent,
	extra func(*component.Registry) error,
) (*v5system.System, repository.Storage, *messengerState, *agentState, *runtimeState) {
	t.Helper()

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
	messengerState := &messengerState{event: event}
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
	if extra != nil {
		if err := extra(registry); err != nil {
			t.Fatalf("register extra component: %v", err)
		}
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

	return v5system.New(storage, profiles, runtimes, registry), storage, messengerState, agentState, runtimeState
}

type mockMessageCommandComponent struct{}

type mockMessagePing struct{}

func (c *mockMessageCommandComponent) Type() string {
	return "tools"
}

func (c *mockMessageCommandComponent) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{{
		ID:      "tools.ping.message",
		Sources: []commandengine.Source{commandengine.SourceMessage},
		Policy:  simplerbac.Public(),
		Routes: []commandengine.Route{{
			Pattern: "tools ping",
			Help:    "Reply with pong",
			Build: func(req *clir.Request) (any, error) {
				_ = req
				return mockMessagePing{}, nil
			},
		}},
	}}
}

func (c *mockMessageCommandComponent) RegisterCommandHandlers(registry *commandengine.Registry) error {
	return commandengine.Register[mockMessagePing](registry, func(ctx context.Context, req commandengine.Request, cmd mockMessagePing) (commandengine.Result, error) {
		_, _, _ = ctx, req, cmd
		return commandengine.Result{Text: "pong"}, nil
	})
}

type noopProcessActions struct{}

func (n *noopProcessActions) Quit(ctx context.Context) error {
	_ = ctx
	return nil
}
