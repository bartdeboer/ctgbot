package integration

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/appstate"
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

		messengerRegistration, err := system.EnsureComponent(ctx, "mockmsg", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockmsg) error = %v", err)
		}
		agentRegistration, err := system.EnsureComponent(ctx, "mockagent", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockagent) error = %v", err)
		}
		commandRegistration, err := system.EnsureComponent(ctx, "tools", "local", "")
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

		messengerRegistration, err := system.EnsureComponent(ctx, "mockmsg", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockmsg) error = %v", err)
		}
		agentRegistration, err := system.EnsureComponent(ctx, "mockagent", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockagent) error = %v", err)
		}
		commandRegistration, err := system.EnsureComponent(ctx, "tools", "local", "")
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

func TestV5AgentBoundCommandSurfaceRunsWithoutSeparateCommandBinding(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		system, storage, messengerState, agentState, runtimeState := newMessageCommandTestSystem(
			t,
			root,
			component.InboundEvent{
				ExternalID: "msg-agent-surface",
				Payload: messenger.InboundPayload{
					ProviderType:      "mockmsg",
					ProviderChatID:    "chat-1",
					ProviderThreadID:  "provider-thread-1",
					ProviderMessageID: "msg-agent-surface",
					Actor:             actorWithRoles("", "bart"),
					Text:              messenger.TextMessage{Text: "/agentctl ping"},
				},
			},
			func(registry *component.Registry) error {
				return registry.Add("agentctl", func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
					_, _, _, _, _ = ctx, registration, runtime, home, storage
					return &mockAgentCommandComponent{}, nil
				})
			},
		)

		messengerRegistration, err := system.EnsureComponent(ctx, "mockmsg", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockmsg) error = %v", err)
		}
		agentRegistration, err := system.EnsureComponent(ctx, "agentctl", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(agentctl) error = %v", err)
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

		b := v5broker.New(storage, system, nil)
		if err := b.Run(ctx); err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if agentState.turnCalls != 0 {
			t.Fatalf("generic mock agent turn calls = %d, want 0", agentState.turnCalls)
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

			messengerRegistration, err := system.EnsureComponent(ctx, "mockmsg", "local", "")
			if err != nil {
				t.Fatalf("EnsureComponent(mockmsg) error = %v", err)
			}
			agentRegistration, err := system.EnsureComponent(ctx, "mockagent", "local", "")
			if err != nil {
				t.Fatalf("EnsureComponent(mockagent) error = %v", err)
			}
			commandRegistration, err := system.EnsureComponent(ctx, v5process.Type, "local", "")
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

			messengerRegistration, err := system.EnsureComponent(ctx, "mockmsg", "local", "")
			if err != nil {
				t.Fatalf("EnsureComponent(mockmsg) error = %v", err)
			}
			agentRegistration, err := system.EnsureComponent(ctx, "mockagent", "local", "")
			if err != nil {
				t.Fatalf("EnsureComponent(mockagent) error = %v", err)
			}
			commandRegistration, err := system.EnsureComponent(ctx, v5process.Type, "local", "")
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

func TestV5ProcessInstallAndUpgradeMessageAliasesAllowOperators(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		cases := []struct {
			text      string
			wantText  string
			wantField string
		}{
			{text: "/install", wantText: "install completed\ntype /quit to restart", wantField: "install"},
			{text: "/process install", wantText: "install completed\ntype /quit to restart", wantField: "install"},
			{text: "/upgrade", wantText: "upgrade completed\ntype /quit to restart", wantField: "upgrade"},
			{text: "/process upgrade", wantText: "upgrade completed\ntype /quit to restart", wantField: "upgrade"},
		}
		for _, tc := range cases {
			actions := &recordingProcessActions{}
			system, storage, messengerState, agentState, runtimeState := newMessageCommandTestSystem(
				t,
				root,
				component.InboundEvent{
					ExternalID: "operator-" + tc.text,
					Payload: messenger.InboundPayload{
						ProviderType:      "mockmsg",
						ProviderChatID:    "chat-1",
						ProviderThreadID:  "provider-thread-1",
						ProviderMessageID: "operator-" + tc.text,
						Actor:             actorWithRoles("13145044", "bart", simplerbac.RoleUser, simplerbac.RoleRoot),
						Text:              messenger.TextMessage{Text: tc.text},
					},
				},
				func(registry *component.Registry) error {
					return registry.Add(v5process.Type, func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
						_, _, _, _, _ = ctx, registration, runtime, home, storage
						return v5process.New(actions), nil
					})
				},
			)

			messengerRegistration, err := system.EnsureComponent(ctx, "mockmsg", "local", "")
			if err != nil {
				t.Fatalf("EnsureComponent(mockmsg) error = %v", err)
			}
			agentRegistration, err := system.EnsureComponent(ctx, "mockagent", "local", "")
			if err != nil {
				t.Fatalf("EnsureComponent(mockagent) error = %v", err)
			}
			commandRegistration, err := system.EnsureComponent(ctx, v5process.Type, "local", "")
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
				t.Fatalf("Run(%q) error = %v", tc.text, err)
			}

			if agentState.turnCalls != 0 {
				t.Fatalf("agent turn calls for %q = %d, want 0", tc.text, agentState.turnCalls)
			}
			if runtimeState.execCalls != 0 {
				t.Fatalf("runtime exec calls for %q = %d, want 0", tc.text, runtimeState.execCalls)
			}
			if got, want := len(messengerState.relayPayloads), 1; got != want {
				t.Fatalf("relay payload count for %q = %d, want %d", tc.text, got, want)
			}
			if got, want := messengerState.relayPayloads[0].Text.Text, tc.wantText; got != want {
				t.Fatalf("relay text for %q = %q, want %q", tc.text, got, want)
			}
			switch tc.wantField {
			case "install":
				if actions.installCalls != 1 || actions.upgradeCalls != 0 || actions.quitCalls != 0 {
					t.Fatalf("process actions for %q = %+v", tc.text, actions)
				}
			case "upgrade":
				if actions.installCalls != 0 || actions.upgradeCalls != 1 || actions.quitCalls != 0 {
					t.Fatalf("process actions for %q = %+v", tc.text, actions)
				}
			}
		}
	})
}

func TestV5HelpListsActiveMessageCommands(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		system, storage, messengerState, agentState, runtimeState := newMessageCommandTestSystem(
			t,
			root,
			component.InboundEvent{
				ExternalID: "help",
				Payload: messenger.InboundPayload{
					ProviderType:      "mockmsg",
					ProviderChatID:    "chat-1",
					ProviderThreadID:  "provider-thread-1",
					ProviderMessageID: "help",
					Actor:             actorWithRoles("", "bart"),
					Text:              messenger.TextMessage{Text: "/help"},
				},
			},
			func(registry *component.Registry) error {
				if err := registry.Add("agentctl", func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
					_, _, _, _, _ = ctx, registration, runtime, home, storage
					return &mockAgentCommandComponent{}, nil
				}); err != nil {
					return err
				}
				return registry.Add(v5process.Type, func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
					_, _, _, _, _ = ctx, registration, runtime, home, storage
					return v5process.New(&noopProcessActions{}), nil
				})
			},
		)

		messengerRegistration, err := system.EnsureComponent(ctx, "mockmsg", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockmsg) error = %v", err)
		}
		agentRegistration, err := system.EnsureComponent(ctx, "agentctl", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(agentctl) error = %v", err)
		}
		commandRegistration, err := system.EnsureComponent(ctx, v5process.Type, "local", "")
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
		text := messengerState.relayPayloads[0].Text.Text
		for _, want := range []string{"/help", "/agentctl ping", "/config get <key>", "/config list", "/config set <key> <value>", "/install", "/process install", "/quit"} {
			if !strings.Contains(text, want) {
				t.Fatalf("help text = %q, missing %q", text, want)
			}
		}
	})
}

func TestV5ConfigMessageCommands(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()
		run := func(text string) string {
			system, storage, messengerState, agentState, runtimeState := newMessageCommandTestSystem(
				t,
				root,
				component.InboundEvent{
					ExternalID: "cfg-" + strings.ReplaceAll(text, " ", "-"),
					Payload: messenger.InboundPayload{
						ProviderType:      "mockmsg",
						ProviderChatID:    "chat-1",
						ProviderThreadID:  "provider-thread-1",
						ProviderMessageID: "cfg-" + strings.ReplaceAll(text, " ", "-"),
						Actor:             actorWithRoles("13145044", "bart", simplerbac.RoleUser, simplerbac.RoleRoot),
						Text:              messenger.TextMessage{Text: text},
					},
				},
				nil,
			)

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
			return messengerState.relayPayloads[0].Text.Text
		}

		if got := run("/config get hostbridge.tcp_listen_addr"); got != "hostbridge.tcp-listen-addr=127.0.0.1:4567" {
			t.Fatalf("config get reply = %q", got)
		}
		if got := run("/config set hostbridge.tcp_listen_addr 127.0.0.1:4568"); got != "hostbridge.tcp-listen-addr=127.0.0.1:4568" {
			t.Fatalf("config set reply = %q", got)
		}
		if got := run("/config get hostbridge.tcp_listen_addr"); got != "hostbridge.tcp-listen-addr=127.0.0.1:4568" {
			t.Fatalf("config get after set reply = %q", got)
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

	system := v5system.New(storage, map[string]v5system.Workspace{}, map[string]v5runtime.Factory{
		"local": fakeRuntimeFactory{
			runtimeKind:    "local",
			rootDir:        root,
			componentsRoot: filepath.Join(root, ".ctgbot", "components"),
			state:          runtimeState,
		},
	}, registry)
	system.StateRoot = filepath.Join(root, ".ctgbot")
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("NewCwd(config store): %v", err)
	}
	system.Config = appstate.New(system.StateRoot, store)

	return system, storage, messengerState, agentState, runtimeState
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

type mockAgentCommandComponent struct{}

type mockAgentCommandPing struct{}

func (c *mockAgentCommandComponent) Type() string {
	return "agentctl"
}

func (c *mockAgentCommandComponent) HandleTurn(ctx context.Context, turn component.Turn) (*component.TurnResult, error) {
	_, _, _ = ctx, turn, c
	return &component.TurnResult{
		Final: &coremodel.ThreadMessage{
			Kind:       coremodel.MessageKindAgent,
			ActorID:    "agentctl",
			ActorLabel: "agentctl",
			Text:       "handled as agent",
		},
	}, nil
}

func (c *mockAgentCommandComponent) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{{
		ID:      "agentctl.ping",
		Sources: []commandengine.Source{commandengine.SourceMessage},
		Policy:  simplerbac.Public(),
		Routes: []commandengine.Route{{
			Pattern: "agentctl ping",
			Help:    "Reply with pong",
			Build: func(req *clir.Request) (any, error) {
				_ = req
				return mockAgentCommandPing{}, nil
			},
		}},
	}}
}

func (c *mockAgentCommandComponent) RegisterCommandHandlers(registry *commandengine.Registry) error {
	return commandengine.Register[mockAgentCommandPing](registry, func(ctx context.Context, req commandengine.Request, cmd mockAgentCommandPing) (commandengine.Result, error) {
		_, _, _ = ctx, req, cmd
		return commandengine.Result{Text: "pong"}, nil
	})
}

type noopProcessActions struct{}

func (n *noopProcessActions) Install(ctx context.Context) error {
	_ = ctx
	return nil
}

func (n *noopProcessActions) Upgrade(ctx context.Context) error {
	_ = ctx
	return nil
}

func (n *noopProcessActions) Quit(ctx context.Context) error {
	_ = ctx
	return nil
}

type recordingProcessActions struct {
	installCalls int
	upgradeCalls int
	quitCalls    int
}

func (r *recordingProcessActions) Install(ctx context.Context) error {
	_ = ctx
	r.installCalls++
	return nil
}

func (r *recordingProcessActions) Upgrade(ctx context.Context) error {
	_ = ctx
	r.upgradeCalls++
	return nil
}

func (r *recordingProcessActions) Quit(ctx context.Context) error {
	_ = ctx
	r.quitCalls++
	return nil
}
