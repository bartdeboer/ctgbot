package integration

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	v5broker "github.com/bartdeboer/ctgbot/internal/v5/broker"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	v5hostbridgeserver "github.com/bartdeboer/ctgbot/internal/v5/hostbridge/server"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
	v5system "github.com/bartdeboer/ctgbot/internal/v5/system"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func init() {
	gob.Register(pingCommand{})
}

func TestV5HostbridgeFlow(t *testing.T) {
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
		bridge := v5hostbridgeserver.NewBridge(root, storage, nil)
		t.Cleanup(func() {
			_ = bridge.Close()
		})

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
				runtime:     runtime.Bind(registration, home, "", nil),
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
		commandRegistration, err := system.EnsureComponent(ctx, "mockcmd", "test")
		if err != nil {
			t.Fatalf("EnsureComponent(mockcmd) error = %v", err)
		}

		if err := system.AuthComponent(ctx, "mockagent", "", 0, 0, io.Discard, io.Discard); err != nil {
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
		ID:      "mockcmd.ping",
		Sources: []commandengine.Source{commandengine.SourceHostbridge},
		Policy:  simplerbac.Public(),
		Routes: []commandengine.Route{{
			Pattern: "mockcmd ping",
			Help:    "Reply with pong",
			Build: func(req *clir.Request) (any, error) {
				_ = req
				return pingCommand{}, nil
			},
		}},
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

func (a *hostbridgeAgent) Type() string {
	return "mockagent"
}

func (a *hostbridgeAgent) Auth(ctx context.Context, callbackPort int, callbackTimeout time.Duration, stdout io.Writer, stderr io.Writer) error {
	_, _, _, _, _ = ctx, callbackPort, callbackTimeout, stdout, stderr
	home := a.runtime.ComponentHome()
	if err := os.MkdirAll(home.HostPath, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(home.HostPath, "auth.json"), []byte(`{"ok":true}`), 0o600); err != nil {
		return err
	}
	a.state.authCalls++
	return nil
}

func (a *hostbridgeAgent) HandleTurn(ctx context.Context, turn component.Turn) (*component.TurnResult, error) {
	if _, err := os.Stat(filepath.Join(a.runtime.ComponentHome().HostPath, "auth.json")); err != nil {
		return nil, fmt.Errorf("missing auth.json: %w", err)
	}

	result, err := a.bridge.DoCommand(ctx, turn.Thread.ID, turn.Runtime.Commands(), commandengine.Request{
		Context: commandengine.Context{
			SandboxID: turn.Thread.ID,
		},
		Command:      pingCommand{},
		DefinitionID: "mockcmd.ping",
		Route:        "mockcmd ping",
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
