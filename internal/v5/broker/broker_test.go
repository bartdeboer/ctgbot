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
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	broker "github.com/bartdeboer/ctgbot/internal/v5/broker"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	v5process "github.com/bartdeboer/ctgbot/internal/v5/component/process"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
	v5system "github.com/bartdeboer/ctgbot/internal/v5/system"
	"github.com/bartdeboer/go-clir"
)

type fakeRuntime struct {
	home v5runtime.Home
	kind string
}

func (r fakeRuntime) Kind() string { return r.kind }
func (r fakeRuntime) ComponentHome() v5runtime.Home {
	return r.home
}
func (r fakeRuntime) RuntimeComponentHomePath() string {
	return r.home.Path
}
func (r fakeRuntime) RuntimeWorkspacePath(workspacePath string) string {
	return workspacePath
}
func (r fakeRuntime) Exec(ctx context.Context, workspacePath string, threadID modeluuid.UUID, commands commandengine.CommandExecutor, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	_, _, _, _, _, _, _, _, _ = ctx, workspacePath, threadID, commands, stdout, stderr, name, args, r.kind
	return fmt.Errorf("not implemented")
}

func (r fakeRuntime) CombinedOutput(ctx context.Context, workspacePath string, threadID modeluuid.UUID, commands commandengine.CommandExecutor, name string, args ...string) ([]byte, error) {
	_, _, _, _, _, _, _ = ctx, workspacePath, threadID, commands, name, args, r.kind
	return nil, fmt.Errorf("not implemented")
}

func (r fakeRuntime) OpenHTTPRelayPort(ctx context.Context, workspacePath string, threadID modeluuid.UUID, commands commandengine.CommandExecutor, callbackPort int, callbackTimeout time.Duration) (func(context.Context) error, error) {
	_, _, _, _, _, _, _ = ctx, workspacePath, threadID, commands, callbackPort, callbackTimeout, r.kind
	return nil, fmt.Errorf("not implemented")
}

type fakeFactory struct {
	kind           string
	rootDir        string
	componentsRoot string
}

func (f fakeFactory) Kind() string { return f.kind }
func (f fakeFactory) ComponentHome(registration coremodel.Component) v5runtime.Home {
	hostPath := registration.HomePath
	if hostPath == "" {
		hostPath = filepath.Join(f.componentsRoot, registration.Type, registration.Name)
	}
	return v5runtime.Home{Path: hostPath}
}
func (f fakeFactory) RuntimeComponentHomePath(registration coremodel.Component, home v5runtime.Home) string {
	_, _ = registration, home
	return home.Path
}
func (f fakeFactory) RuntimeWorkspacePath(workspacePath string) string {
	return workspacePath
}
func (f fakeFactory) Bind(registration coremodel.Component, home v5runtime.Home, image string, env []string) v5runtime.Runtime {
	_, _, _ = registration, image, env
	return fakeRuntime{
		home: home,
		kind: f.kind,
	}
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
	prompts    []string
	homes      []v5runtime.Home
	streamText string
	finalText  string
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
	streamText := strings.TrimSpace(c.recorder.streamText)
	if streamText == "" {
		streamText = "working..."
	}
	if err := turn.Runtime.Send(context.Background(), messenger.OutboundPayload{
		Text: messenger.TextMessage{Text: streamText},
	}); err != nil {
		return nil, err
	}
	finalText := strings.TrimSpace(c.recorder.finalText)
	if finalText == "" {
		finalText = "done"
	}
	return &component.TurnResult{
		Final: &coremodel.ThreadMessage{Text: finalText},
	}, nil
}

func newTestSystem(t *testing.T, root string, storage repository.Storage, recorder *fakeMessengerRecorder, agentRecorder *fakeAgentRecorder, events []component.InboundEvent, extras ...func(*component.Registry) error) *v5system.System {
	t.Helper()

	registry := component.NewRegistry()
	if err := registry.Add("telegram", func(ctx context.Context, registration coremodel.Component, rt v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
		_, _, _, _, _ = ctx, rt, home, storage, registration
		return &fakeMessenger{componentID: registration.ID, recorder: recorder, events: append([]component.InboundEvent(nil), events...)}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add("codex", func(ctx context.Context, registration coremodel.Component, rt v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
		_, _, _, _, _ = ctx, rt, home, storage, registration
		return &fakeAgent{componentID: registration.ID, recorder: agentRecorder}, nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, extra := range extras {
		if err := extra(registry); err != nil {
			t.Fatal(err)
		}
	}

	system := v5system.New(
		storage,
		map[string]v5system.Workspace{},
		map[string]v5runtime.Factory{"local": fakeFactory{kind: "local", rootDir: root, componentsRoot: filepath.Join(root, ".ctgbot", "components")}},
		registry,
	)
	system.StateRoot = filepath.Join(root, ".ctgbot")
	return system
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
	telegram := &coremodel.Component{Type: "telegram", Name: "telegram", Runtime: "local", Enabled: true, IsDefault: true}
	codex := &coremodel.Component{Type: "codex", Name: "codex", Runtime: "local", Enabled: true, IsDefault: true}
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
			Actor: messenger.Actor{
				ID:    "bart",
				Label: "bart",
				Roles: []simplerbac.Role{simplerbac.RoleUser},
			},
			Text: messenger.TextMessage{Text: "hello"},
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
	if agentRecorder.homes[0].Path == "" || !strings.Contains(agentRecorder.homes[0].Path, filepath.Join(".ctgbot", "components", "codex", "codex")) {
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

func TestHandleInboundSuppressesFinalReplyAlreadySentByAgentOutput(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	messengerRecorder := &fakeMessengerRecorder{}
	agentRecorder := &fakeAgentRecorder{streamText: "done", finalText: "done"}
	system := newTestSystem(t, root, storage, messengerRecorder, agentRecorder, nil)
	b := broker.New(storage, system, nil)

	chat := &coremodel.Chat{Label: "team", Enabled: true}
	if err := storage.Chats().Save(context.Background(), chat); err != nil {
		t.Fatal(err)
	}
	telegram := &coremodel.Component{Type: "telegram", Name: "telegram", Runtime: "local", Enabled: true, IsDefault: true}
	codex := &coremodel.Component{Type: "codex", Name: "codex", Runtime: "local", Enabled: true, IsDefault: true}
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
			Actor: messenger.Actor{
				ID:    "bart",
				Label: "bart",
				Roles: []simplerbac.Role{simplerbac.RoleUser},
			},
			Text: messenger.TextMessage{Text: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	if got, want := len(messengerRecorder.payloads), 1; got != want {
		t.Fatalf("relay payloads = %d, want %d", got, want)
	}
	if messengerRecorder.payloads[0].Text.Text != "done" {
		t.Fatalf("relay texts = %#v", messengerRecorder.payloads)
	}
	if got, want := len(outcome.Outbound), 1; got != want {
		t.Fatalf("outbound messages = %d, want %d", got, want)
	}
	messages, err := storage.Messages().ListByThreadID(context.Background(), outcome.Inbound.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(messages), 2; got != want {
		t.Fatalf("stored messages = %d, want %d", got, want)
	}
}

func TestHandleInboundRunsMessageCommandAndSkipsAgent(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	messengerRecorder := &fakeMessengerRecorder{}
	agentRecorder := &fakeAgentRecorder{}
	commandRecorder := &fakeCommandRecorder{}
	system := newTestSystem(
		t,
		root,
		storage,
		messengerRecorder,
		agentRecorder,
		nil,
		func(registry *component.Registry) error {
			return registry.Add("tools", func(ctx context.Context, registration coremodel.Component, rt v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
				_, _, _, _, _ = ctx, rt, home, storage, registration
				return &fakeCommandComponent{recorder: commandRecorder}, nil
			})
		},
	)
	b := broker.New(storage, system, nil)

	chat := &coremodel.Chat{Label: "team", Enabled: true}
	if err := storage.Chats().Save(context.Background(), chat); err != nil {
		t.Fatal(err)
	}
	telegram := &coremodel.Component{Type: "telegram", Name: "telegram", Runtime: "local", Enabled: true, IsDefault: true}
	codex := &coremodel.Component{Type: "codex", Name: "codex", Runtime: "local", Enabled: true, IsDefault: true}
	tools := &coremodel.Component{Type: "tools", Name: "tools", Runtime: "local", Enabled: true, IsDefault: true}
	for _, registration := range []*coremodel.Component{telegram, codex, tools} {
		if err := storage.Components().Save(context.Background(), registration); err != nil {
			t.Fatal(err)
		}
	}
	for _, binding := range []coremodel.ChatComponent{
		{ChatID: chat.ID, ComponentID: telegram.ID, Role: coremodel.ChatComponentRoleSource, ExternalChatID: "chat-1", Enabled: true},
		{ChatID: chat.ID, ComponentID: telegram.ID, Role: coremodel.ChatComponentRoleRelay, ExternalChatID: "chat-1", Enabled: true},
		{ChatID: chat.ID, ComponentID: codex.ID, Role: coremodel.ChatComponentRoleAgent, Enabled: true},
		{ChatID: chat.ID, ComponentID: tools.ID, Role: coremodel.ChatComponentRoleCommand, Enabled: true},
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
			Actor: messenger.Actor{
				ID:    "bart",
				Label: "bart",
				Roles: []simplerbac.Role{simplerbac.RoleUser},
			},
			Text: messenger.TextMessage{Text: "/tools ping"},
		},
	})
	if err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	if outcome.Dropped {
		t.Fatal("expected routed event, got dropped")
	}
	if got, want := len(agentRecorder.prompts), 0; got != want {
		t.Fatalf("agent prompts = %d, want %d", got, want)
	}
	if commandRecorder.calls != 1 {
		t.Fatalf("command calls = %d, want 1", commandRecorder.calls)
	}
	if got, want := len(messengerRecorder.payloads), 1; got != want {
		t.Fatalf("relay payloads = %d, want %d", got, want)
	}
	if got, want := messengerRecorder.payloads[0].Text.Text, "pong"; got != want {
		t.Fatalf("relay text = %q, want %q", got, want)
	}
	if outcome.Inbound != nil {
		t.Fatalf("expected command to stay out of history, got inbound %#v", outcome.Inbound)
	}
	if got, want := len(outcome.Outbound), 1; got != want {
		t.Fatalf("outbound count = %d, want %d", got, want)
	}
	if got, want := outcome.Outbound[0].Kind, coremodel.MessageKindSystem; got != want {
		t.Fatalf("outbound kind = %q, want %q", got, want)
	}
	messages, err := storage.Messages().ListByThreadID(context.Background(), outcome.Outbound[0].ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(messages), 1; got != want {
		t.Fatalf("stored messages = %d, want %d", got, want)
	}
	if got, want := messages[0].Text, "pong"; got != want {
		t.Fatalf("stored message text = %q, want %q", got, want)
	}
}

func TestHandleInboundRecognizesProcessQuitAliasAndSkipsAgent(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	messengerRecorder := &fakeMessengerRecorder{}
	agentRecorder := &fakeAgentRecorder{}
	system := newTestSystem(
		t,
		root,
		storage,
		messengerRecorder,
		agentRecorder,
		nil,
		func(registry *component.Registry) error {
			return registry.Add(v5process.Type, func(ctx context.Context, registration coremodel.Component, rt v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
				_, _, _, _, _ = ctx, registration, rt, home, storage
				return v5process.New(&fakeProcessActions{}), nil
			})
		},
	)
	b := broker.New(storage, system, nil)

	chat := &coremodel.Chat{Label: "team", Enabled: true}
	if err := storage.Chats().Save(context.Background(), chat); err != nil {
		t.Fatal(err)
	}
	telegram := &coremodel.Component{Type: "telegram", Name: "telegram", Runtime: "local", Enabled: true, IsDefault: true}
	codex := &coremodel.Component{Type: "codex", Name: "codex", Runtime: "local", Enabled: true, IsDefault: true}
	process := &coremodel.Component{Type: v5process.Type, Name: v5process.Type, Runtime: "local", Enabled: true, IsDefault: true}
	for _, registration := range []*coremodel.Component{telegram, codex, process} {
		if err := storage.Components().Save(context.Background(), registration); err != nil {
			t.Fatal(err)
		}
	}
	for _, binding := range []coremodel.ChatComponent{
		{ChatID: chat.ID, ComponentID: telegram.ID, Role: coremodel.ChatComponentRoleSource, ExternalChatID: "chat-1", Enabled: true},
		{ChatID: chat.ID, ComponentID: telegram.ID, Role: coremodel.ChatComponentRoleRelay, ExternalChatID: "chat-1", Enabled: true},
		{ChatID: chat.ID, ComponentID: codex.ID, Role: coremodel.ChatComponentRoleAgent, Enabled: true},
		{ChatID: chat.ID, ComponentID: process.ID, Role: coremodel.ChatComponentRoleCommand, Enabled: true},
	} {
		binding := binding
		if err := storage.ChatComponents().Save(context.Background(), &binding); err != nil {
			t.Fatal(err)
		}
	}

	for _, text := range []string{"/quit", "/process quit"} {
		outcome, err := b.HandleInbound(context.Background(), component.InboundEvent{
			ComponentID: telegram.ID,
			ExternalID:  "msg-" + strings.ReplaceAll(text, " ", "-"),
			Payload: messenger.InboundPayload{
				ProviderType:      "telegram",
				ProviderChatID:    "chat-1",
				ProviderThreadID:  "thread-7",
				ProviderMessageID: "msg-" + strings.ReplaceAll(text, " ", "-"),
				Actor: messenger.Actor{
					ID:    "bart",
					Label: "bart",
					Roles: []simplerbac.Role{simplerbac.RoleUser},
				},
				Text: messenger.TextMessage{Text: text},
			},
		})
		if err != nil {
			t.Fatalf("HandleInbound(%q) error = %v", text, err)
		}
		if outcome.Dropped {
			t.Fatalf("expected %q to be routed, got dropped", text)
		}
	}

	if got, want := len(agentRecorder.prompts), 0; got != want {
		t.Fatalf("agent prompts = %d, want %d", got, want)
	}
	if got, want := len(messengerRecorder.payloads), 2; got != want {
		t.Fatalf("relay payloads = %d, want %d", got, want)
	}
	for _, payload := range messengerRecorder.payloads {
		if !strings.Contains(payload.Text.Text, "command error:") {
			t.Fatalf("expected command error relay, got %#v", payload)
		}
	}
	threads, err := storage.Threads().ListByChatID(context.Background(), chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(threads), 1; got != want {
		t.Fatalf("thread count = %d, want %d", got, want)
	}
	messages, err := storage.Messages().ListByThreadID(context.Background(), threads[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(messages), 2; got != want {
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
			Actor: messenger.Actor{
				ID:    "bart",
				Label: "bart",
				Roles: []simplerbac.Role{simplerbac.RoleUser},
			},
			Text: messenger.TextMessage{Text: "ping"},
		},
	}})
	b := broker.New(storage, system, nil)

	chat := &coremodel.Chat{Label: "team", Enabled: true}
	if err := storage.Chats().Save(context.Background(), chat); err != nil {
		t.Fatal(err)
	}
	telegram := &coremodel.Component{Type: "telegram", Name: "telegram", Runtime: "local", Enabled: true, IsDefault: true}
	codex := &coremodel.Component{Type: "codex", Name: "codex", Runtime: "local", Enabled: true, IsDefault: true}
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

type fakeCommandRecorder struct {
	calls int
}

type fakeCommandComponent struct {
	recorder *fakeCommandRecorder
}

type pingCommand struct{}

func (c *fakeCommandComponent) Type() string { return "tools" }

func (c *fakeCommandComponent) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{{
		ID:      "tools.ping.message",
		Sources: []commandengine.Source{commandengine.SourceMessage},
		Policy:  simplerbac.Public(),
		Routes: []commandengine.Route{{
			Pattern: "tools ping",
			Help:    "Reply with pong",
			Build: func(req *clir.Request) (any, error) {
				_ = req
				return pingCommand{}, nil
			},
		}},
	}}
}

func (c *fakeCommandComponent) RegisterCommandHandlers(registry *commandengine.Registry) error {
	return commandengine.Register[pingCommand](registry, func(ctx context.Context, req commandengine.Request, cmd pingCommand) (commandengine.Result, error) {
		_, _, _ = ctx, req, cmd
		c.recorder.calls++
		return commandengine.Result{Text: "pong"}, nil
	})
}

type fakeProcessActions struct{}

func (f *fakeProcessActions) Quit(ctx context.Context) error {
	_ = ctx
	return nil
}
