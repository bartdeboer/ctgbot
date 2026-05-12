package integration

import (
	"context"
	"path/filepath"
	"testing"

	brokerpkg "github.com/bartdeboer/ctgbot/internal/broker"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	systempkg "github.com/bartdeboer/ctgbot/internal/system"
)

func TestGmailSourceRelaysOutboundElsewhere(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()

		storage := newSQLiteStorage(t)
		runtimeState := &runtimeState{}
		gmailState := &gmailSourceState{
			event: component.InboundEvent{
				ExternalID: "gmail-msg-1",
				Payload: message.InboundPayload{
					ProviderType:      "mockgmail",
					ProviderChannelID: "gmail-inbox-1",
					ProviderThreadID:  "gmail-thread-1",
					ProviderMessageID: "gmail-msg-1",
					Actor:             actorWithRoles("", "bart@example.com"),
					Text:              message.TextMessage{Text: "hello from gmail"},
				},
			},
		}
		relayState := &relayState{}
		agentState := &agentState{}

		registry := component.NewRegistry()
		if err := registry.Add("mockgmail", func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
			_, _, _, _, _ = ctx, runtime, home, storage, registration
			return &mockGmailSource{
				componentID: registration.ID,
				state:       gmailState,
			}, nil
		}); err != nil {
			t.Fatalf("register mockgmail: %v", err)
		}
		if err := registry.Add("mockrelay", func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
			_, _, _, _, _ = ctx, runtime, home, storage, registration
			return &mockRelay{
				state: relayState,
			}, nil
		}); err != nil {
			t.Fatalf("register mockrelay: %v", err)
		}
		if err := registry.Add("mockagent", func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
			_, _, _ = ctx, home, storage
			return &mockAgent{
				componentID: registration.ID,
				runtime:     runtime.Bind(registration, home, runtimepkg.BindConfig{}),
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

		gmailRegistration, err := system.EnsureComponent(ctx, "mockgmail", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockgmail) error = %v", err)
		}
		relayRegistration, err := system.EnsureComponent(ctx, "mockrelay", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockrelay) error = %v", err)
		}
		agentRegistration, err := system.EnsureComponent(ctx, "mockagent", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockagent) error = %v", err)
		}

		if err := system.AuthComponent(ctx, "mockagent", "", "", 0, 0, nil, nil); err != nil {
			t.Fatalf("AuthComponent() error = %v", err)
		}

		chat := &coremodel.Chat{Label: "team", Enabled: true}
		if err := storage.Chats().Save(ctx, chat); err != nil {
			t.Fatalf("Chats().Save() error = %v", err)
		}

		if _, err := system.BindChatComponent(ctx, chat.ID, coremodel.ChatComponentRoleSource, gmailRegistration.Ref(), "gmail-inbox-1"); err != nil {
			t.Fatalf("BindChatComponent(source) error = %v", err)
		}
		if _, err := system.BindChatComponent(ctx, chat.ID, coremodel.ChatComponentRoleRelay, relayRegistration.Ref(), "telegram-chat-1"); err != nil {
			t.Fatalf("BindChatComponent(relay) error = %v", err)
		}
		if _, err := system.BindChatComponent(ctx, chat.ID, coremodel.ChatComponentRoleAgent, agentRegistration.Ref(), ""); err != nil {
			t.Fatalf("BindChatComponent(agent) error = %v", err)
		}

		b := brokerpkg.NewWithDeps(storage, system, nil, nil)
		if err := b.Run(ctx); err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if gmailState.runCalls != 1 {
			t.Fatalf("gmail run calls = %d, want 1", gmailState.runCalls)
		}
		if agentState.turnCalls != 1 {
			t.Fatalf("agent turn calls = %d, want 1", agentState.turnCalls)
		}
		if agentState.prompt != "hello from gmail" {
			t.Fatalf("prompt = %q, want hello from gmail", agentState.prompt)
		}
		if runtimeState.execCalls != 1 {
			t.Fatalf("runtime exec calls = %d, want 1", runtimeState.execCalls)
		}
		if len(relayState.payloads) != 1 {
			t.Fatalf("relay payload count = %d, want 1", len(relayState.payloads))
		}
		payload := relayState.payloads[0]
		if payload.Text.Text != "done" {
			t.Fatalf("relay text = %q, want done", payload.Text.Text)
		}
		if payload.ProviderChannelID != "telegram-chat-1" {
			t.Fatalf("relay provider channel id = %q, want telegram-chat-1", payload.ProviderChannelID)
		}
		if payload.ProviderChannelID == "gmail-inbox-1" {
			t.Fatalf("relay unexpectedly targeted gmail inbox: %#v", payload)
		}
		if payload.ProviderThreadID != "" {
			t.Fatalf("relay provider thread id = %q, want empty fallback", payload.ProviderThreadID)
		}
	})
}

type gmailSourceState struct {
	runCalls int
	event    component.InboundEvent
}

type mockGmailSource struct {
	componentID modeluuid.UUID
	state       *gmailSourceState
}

func (m *mockGmailSource) Type() string {
	return "mockgmail"
}

func (m *mockGmailSource) RunInbound(ctx context.Context, emit component.InboundEmitter) error {
	m.state.runCalls++
	event := m.state.event
	event.ComponentID = m.componentID
	return emit(ctx, event)
}

type relayState struct {
	payloads []message.OutboundPayload
}

type mockRelay struct {
	state *relayState
}

func (m *mockRelay) Type() string {
	return "mockrelay"
}

func (m *mockRelay) Send(ctx context.Context, payload message.OutboundPayload) error {
	_ = ctx
	m.state.payloads = append(m.state.payloads, payload)
	return nil
}

func (m *mockRelay) StartChatAction(ctx context.Context, target message.ChatTarget, action message.ChatAction) (func(), error) {
	_, _, _ = ctx, target, action
	return func() {}, nil
}
