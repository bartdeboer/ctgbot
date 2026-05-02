package broker_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	v3component "github.com/bartdeboer/ctgbot/internal/v3/component"
	"github.com/bartdeboer/ctgbot/internal/v3/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v3/homes"
	"github.com/bartdeboer/ctgbot/internal/v3/repository"
	v3runtime "github.com/bartdeboer/ctgbot/internal/v3/runtime"
)

type fakeMessengerFactory struct {
	recorder *fakeMessengerRecorder
	events   []v3component.InboundEvent
}

func (f *fakeMessengerFactory) Type() string { return "telegram" }

func (f *fakeMessengerFactory) Create(ctx context.Context, req v3component.CreateRequest) (v3component.Component, error) {
	_ = ctx
	return &fakeMessengerComponent{
		componentType: req.Registration.Type,
		componentID:   req.Registration.ID,
		recorder:      f.recorder,
		events:        append([]v3component.InboundEvent(nil), f.events...),
	}, nil
}

type fakeMessengerComponent struct {
	componentType string
	componentID   modeluuid.UUID
	recorder      *fakeMessengerRecorder
	events        []v3component.InboundEvent
}

func (c *fakeMessengerComponent) Type() string { return c.componentType }

func (c *fakeMessengerComponent) RunInbound(ctx context.Context, emit v3component.InboundEmitter) error {
	for _, event := range c.events {
		event.ComponentID = c.componentID
		if err := emit(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (c *fakeMessengerComponent) Send(ctx context.Context, payload messenger.OutboundPayload) error {
	_ = ctx
	c.recorder.payloads = append(c.recorder.payloads, payload)
	return nil
}

func (c *fakeMessengerComponent) StartChatAction(ctx context.Context, target messenger.ChatTarget, action messenger.ChatAction) (func(), error) {
	_ = ctx
	c.recorder.actions = append(c.recorder.actions, string(action)+"@"+target.ProviderChatID+":"+target.ProviderThreadID)
	return func() {}, nil
}

type fakeMessengerRecorder struct {
	payloads []messenger.OutboundPayload
	actions  []string
}

type fakeAgentFactory struct {
	recorder *fakeAgentRecorder
}

func (f *fakeAgentFactory) Type() string { return "codex" }

func (f *fakeAgentFactory) Create(ctx context.Context, req v3component.CreateRequest) (v3component.Component, error) {
	_ = ctx
	return &fakeAgentComponent{
		componentID: req.Registration.ID,
		home:        req.Home,
		recorder:    f.recorder,
	}, nil
}

type fakeAgentComponent struct {
	componentID modeluuid.UUID
	home        v3component.Home
	recorder    *fakeAgentRecorder
}

func (c *fakeAgentComponent) Type() string { return "codex" }

func (c *fakeAgentComponent) HandleTurn(ctx context.Context, turn v3component.Turn) (*v3component.TurnResult, error) {
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
	return &v3component.TurnResult{
		Final: &coremodel.ThreadMessage{
			Text: "done",
		},
	}, nil
}

type fakeAgentRecorder struct {
	prompts []string
	homes   []v3component.Home
}

func TestHandleInboundRoutesThroughBoundAgentAndRelay(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	messengerRecorder := &fakeMessengerRecorder{}
	agentRecorder := &fakeAgentRecorder{}
	registry := v3component.NewRegistry(
		&fakeMessengerFactory{recorder: messengerRecorder},
		&fakeAgentFactory{recorder: agentRecorder},
	)
	rt := v3runtime.New(storage, registry, homes.New(root))
	b := rt.Broker(nil)

	chat := &coremodel.Chat{Label: "team", Enabled: true}
	if err := storage.Chats().Save(context.Background(), chat); err != nil {
		t.Fatal(err)
	}
	telegram := &coremodel.Component{Type: "telegram", Name: "telegram", Enabled: true, IsDefault: true}
	codex := &coremodel.Component{Type: "codex", Name: "codex", Enabled: true, IsDefault: true}
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

	outcome, err := b.HandleInbound(context.Background(), v3component.InboundEvent{
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
	if agentRecorder.homes[0].HostPath == "" || !strings.Contains(agentRecorder.homes[0].HostPath, filepath.Join(".ctgbot", "components", "codex", "codex")) {
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
	registry := v3component.NewRegistry(
		&fakeMessengerFactory{
			recorder: messengerRecorder,
			events: []v3component.InboundEvent{{
				ExternalID: "msg-2",
				Payload: messenger.InboundPayload{
					ProviderType:      "telegram",
					ProviderChatID:    "chat-2",
					ProviderThreadID:  "thread-9",
					ProviderMessageID: "msg-2",
					UserLabel:         "bart",
					Text:              messenger.TextMessage{Text: "ping"},
				},
			}},
		},
		&fakeAgentFactory{recorder: agentRecorder},
	)
	rt := v3runtime.New(storage, registry, homes.New(root))
	b := rt.Broker(nil)

	chat := &coremodel.Chat{Label: "team", Enabled: true}
	_ = storage.Chats().Save(context.Background(), chat)
	telegram := &coremodel.Component{Type: "telegram", Name: "telegram", Enabled: true, IsDefault: true}
	codex := &coremodel.Component{Type: "codex", Name: "codex", Enabled: true, IsDefault: true}
	_ = storage.Components().Save(context.Background(), telegram)
	_ = storage.Components().Save(context.Background(), codex)
	for _, binding := range []coremodel.ChatComponent{
		{ChatID: chat.ID, ComponentID: telegram.ID, Role: coremodel.ChatComponentRoleSource, ExternalChatID: "chat-2", Enabled: true},
		{ChatID: chat.ID, ComponentID: telegram.ID, Role: coremodel.ChatComponentRoleRelay, ExternalChatID: "chat-2", Enabled: true},
		{ChatID: chat.ID, ComponentID: codex.ID, Role: coremodel.ChatComponentRoleAgent, Enabled: true},
	} {
		binding := binding
		_ = storage.ChatComponents().Save(context.Background(), &binding)
	}

	if err := b.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := len(messengerRecorder.payloads), 2; got != want {
		t.Fatalf("relay payloads = %d, want %d", got, want)
	}
}
