package broker_test

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	broker "github.com/bartdeboer/ctgbot/internal/v5/broker"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
	v5system "github.com/bartdeboer/ctgbot/internal/v5/system"
)

type fakeRuntime struct {
	profile v5runtime.Profile
	rootDir string
}

func (r fakeRuntime) Kind() string               { return r.profile.Runtime }
func (r fakeRuntime) Profile() v5runtime.Profile { return r.profile }
func (r fakeRuntime) ComponentHome(registration coremodel.Component) v5runtime.Home {
	return v5runtime.Home{
		HostPath:      filepath.Join(r.profile.Root, "components", registration.Type, registration.Name),
		ContainerPath: "/profile/components/" + registration.Type + "/" + registration.Name,
	}
}
func (r fakeRuntime) ThreadWorkspace(threadID modeluuid.UUID) (string, string, error) {
	return filepath.Join(r.rootDir, ".ctgbot", "threads", threadID.String(), "workspace"), "/workspace", nil
}

func (r fakeRuntime) Exec(ctx context.Context, registration coremodel.Component, threadID modeluuid.UUID, home v5runtime.Home, image string, workdir string, env []string, developerInstructions string, commands commandengine.CommandExecutor, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	_, _, _, _, _, _, _, _, _, _, _, _ = ctx, registration, threadID, home, image, workdir, env, developerInstructions, commands, stdout, stderr, name
	_ = args
	return fmt.Errorf("not implemented")
}

func (r fakeRuntime) CombinedOutput(ctx context.Context, registration coremodel.Component, threadID modeluuid.UUID, home v5runtime.Home, image string, workdir string, env []string, developerInstructions string, commands commandengine.CommandExecutor, name string, args ...string) ([]byte, error) {
	_, _, _, _, _, _, _, _, _, _ = ctx, registration, threadID, home, image, workdir, env, developerInstructions, commands, name
	_ = args
	return nil, fmt.Errorf("not implemented")
}

func (r fakeRuntime) OpenHTTPRelayPort(ctx context.Context, registration coremodel.Component, threadID modeluuid.UUID, home v5runtime.Home, image string, workdir string, env []string, developerInstructions string, commands commandengine.CommandExecutor, callbackPort int, callbackTimeout time.Duration) (func(context.Context) error, error) {
	_, _, _, _, _, _, _, _, _, _, _ = ctx, registration, threadID, home, image, workdir, env, developerInstructions, commands, callbackPort, callbackTimeout
	return nil, fmt.Errorf("not implemented")
}

type fakeMessengerRecorder struct {
	payloads []messenger.OutboundPayload
}

type fakeMessenger struct {
	componentID modeluuid.UUID
	recorder    *fakeMessengerRecorder
	events      []component.InboundEvent
}

func (c *fakeMessenger) Type() string { return "telegram" }
func (c *fakeMessenger) RunInbound(ctx context.Context, emit component.InboundEmitter) error {
	for _, event := range c.events {
		event.ComponentID = c.componentID
		if err := emit(ctx, event); err != nil {
			return err
		}
	}
	return nil
}
func (c *fakeMessenger) Send(ctx context.Context, payload messenger.OutboundPayload) error {
	_ = ctx
	c.recorder.payloads = append(c.recorder.payloads, payload)
	return nil
}
func (c *fakeMessenger) StartChatAction(ctx context.Context, target messenger.ChatTarget, action messenger.ChatAction) (func(), error) {
	_, _, _ = ctx, target, action
	return func() {}, nil
}

type fakeAgentRecorder struct {
	prompts []string
	homes   []v5runtime.Home
}

type fakeAgent struct {
	componentID modeluuid.UUID
	recorder    *fakeAgentRecorder
}

func (c *fakeAgent) Type() string { return "codex" }
func (c *fakeAgent) HandleTurn(ctx context.Context, turn component.Turn) (*component.TurnResult, error) {
	_ = ctx
	c.recorder.prompts = append(c.recorder.prompts, turn.Inbound.Text)
	if home, ok := turn.Runtime.ComponentHome(c.componentID); ok {
		c.recorder.homes = append(c.recorder.homes, home)
	}
	if err := turn.Runtime.Send(context.Background(), messenger.OutboundPayload{
		Text: messenger.TextMessage{Text: "working..."},
	}); err != nil {
		return nil, err
	}
	return &component.TurnResult{
		Final: &coremodel.ThreadMessage{Text: "done"},
	}, nil
}

func newTestSystem(t *testing.T, root string, storage repository.Storage, recorder *fakeMessengerRecorder, agentRecorder *fakeAgentRecorder, events []component.InboundEvent) *v5system.System {
	t.Helper()

	registry := component.NewRegistry()
	if err := registry.Add("telegram", func(ctx context.Context, registration coremodel.Component, profile v5runtime.Profile, rt v5runtime.Runtime, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
		_, _, _, _, _, _ = ctx, profile, rt, home, storage, registration
		return &fakeMessenger{componentID: registration.ID, recorder: recorder, events: append([]component.InboundEvent(nil), events...)}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add("codex", func(ctx context.Context, registration coremodel.Component, profile v5runtime.Profile, rt v5runtime.Runtime, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
		_, _, _, _, _, _ = ctx, profile, rt, home, storage, registration
		return &fakeAgent{componentID: registration.ID, recorder: agentRecorder}, nil
	}); err != nil {
		t.Fatal(err)
	}

	profile := v5runtime.Profile{Name: "default", Runtime: "local", Root: filepath.Join(root, ".ctgbot", "profiles", "default")}
	return v5system.New(
		storage,
		map[string]v5runtime.Profile{"default": profile},
		map[string]v5runtime.Runtime{"default": fakeRuntime{profile: profile, rootDir: root}},
		registry,
	)
}

func TestHandleInboundRoutesThroughBoundAgentAndRelay(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	messengerRecorder := &fakeMessengerRecorder{}
	agentRecorder := &fakeAgentRecorder{}
	system := newTestSystem(t, root, storage, messengerRecorder, agentRecorder, nil)
	b := broker.New(storage, system, nil)

	chat := &coremodel.Chat{Label: "team", Enabled: true}
	if err := storage.Chats().Save(context.Background(), chat); err != nil {
		t.Fatal(err)
	}
	telegram := &coremodel.Component{Type: "telegram", Name: "telegram", Profile: "default", Enabled: true, IsDefault: true}
	codex := &coremodel.Component{Type: "codex", Name: "codex", Profile: "default", Enabled: true, IsDefault: true}
	if err := storage.Components().Save(context.Background(), telegram); err != nil {
		t.Fatal(err)
	}
	if err := storage.Components().Save(context.Background(), codex); err != nil {
		t.Fatal(err)
	}
	for _, binding := range []coremodel.ChatComponent{
		{ChatID: chat.ID, ComponentID: telegram.ID, Role: coremodel.ChatComponentRoleSource, ExternalChatID: "chat-1", Enabled: true},
		{ChatID: chat.ID, ComponentID: telegram.ID, Role: coremodel.ChatComponentRoleRelay, ExternalChatID: "chat-1", Enabled: true},
		{ChatID: chat.ID, ComponentID: codex.ID, Role: coremodel.ChatComponentRoleAgent, Enabled: true},
	} {
		binding := binding
		if err := storage.ChatComponents().Save(context.Background(), &binding); err != nil {
			t.Fatal(err)
		}
	}

	outcome, err := b.HandleInbound(context.Background(), component.InboundEvent{
		ComponentID: telegram.ID,
		ExternalID:  "msg-1",
		Payload: messenger.InboundPayload{
			ProviderType:      "telegram",
			ProviderChatID:    "chat-1",
			ProviderThreadID:  "thread-7",
			ProviderMessageID: "msg-1",
			UserLabel:         "bart",
			Text:              messenger.TextMessage{Text: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	if outcome.Dropped {
		t.Fatal("expected routed event, got dropped")
	}
	if got, want := len(agentRecorder.prompts), 1; got != want {
		t.Fatalf("agent prompts = %d, want %d", got, want)
	}
	if agentRecorder.prompts[0] != "hello" {
		t.Fatalf("agent prompt = %q", agentRecorder.prompts[0])
	}
	if got, want := len(messengerRecorder.payloads), 2; got != want {
		t.Fatalf("relay payloads = %d, want %d", got, want)
	}
	if messengerRecorder.payloads[0].Text.Text != "working..." || messengerRecorder.payloads[1].Text.Text != "done" {
		t.Fatalf("relay texts = %#v", messengerRecorder.payloads)
	}
	if agentRecorder.homes[0].HostPath == "" || !strings.Contains(agentRecorder.homes[0].HostPath, filepath.Join(".ctgbot", "profiles", "default", "components", "codex", "codex")) {
		t.Fatalf("agent home = %#v", agentRecorder.homes[0])
	}
	messages, err := storage.Messages().ListByThreadID(context.Background(), outcome.Inbound.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(messages), 3; got != want {
		t.Fatalf("stored messages = %d, want %d", got, want)
	}
}

func TestRunStartsEnabledInboundSources(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	messengerRecorder := &fakeMessengerRecorder{}
	agentRecorder := &fakeAgentRecorder{}
	system := newTestSystem(t, root, storage, messengerRecorder, agentRecorder, []component.InboundEvent{{
		ExternalID: "msg-2",
		Payload: messenger.InboundPayload{
			ProviderType:      "telegram",
			ProviderChatID:    "chat-2",
			ProviderThreadID:  "thread-9",
			ProviderMessageID: "msg-2",
			UserLabel:         "bart",
			Text:              messenger.TextMessage{Text: "ping"},
		},
	}})
	b := broker.New(storage, system, nil)

	chat := &coremodel.Chat{Label: "team", Enabled: true}
	if err := storage.Chats().Save(context.Background(), chat); err != nil {
		t.Fatal(err)
	}
	telegram := &coremodel.Component{Type: "telegram", Name: "telegram", Profile: "default", Enabled: true, IsDefault: true}
	codex := &coremodel.Component{Type: "codex", Name: "codex", Profile: "default", Enabled: true, IsDefault: true}
	if err := storage.Components().Save(context.Background(), telegram); err != nil {
		t.Fatal(err)
	}
	if err := storage.Components().Save(context.Background(), codex); err != nil {
		t.Fatal(err)
	}
	for _, binding := range []coremodel.ChatComponent{
		{ChatID: chat.ID, ComponentID: telegram.ID, Role: coremodel.ChatComponentRoleSource, ExternalChatID: "chat-2", Enabled: true},
		{ChatID: chat.ID, ComponentID: telegram.ID, Role: coremodel.ChatComponentRoleRelay, ExternalChatID: "chat-2", Enabled: true},
		{ChatID: chat.ID, ComponentID: codex.ID, Role: coremodel.ChatComponentRoleAgent, Enabled: true},
	} {
		binding := binding
		if err := storage.ChatComponents().Save(context.Background(), &binding); err != nil {
			t.Fatal(err)
		}
	}

	if err := b.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := len(messengerRecorder.payloads), 2; got != want {
		t.Fatalf("relay payloads = %d, want %d", got, want)
	}
}
