package broker_test

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	broker "github.com/bartdeboer/ctgbot/internal/broker"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	processcomponent "github.com/bartdeboer/ctgbot/internal/component/process"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	systempkg "github.com/bartdeboer/ctgbot/internal/system"
	"github.com/bartdeboer/go-clir"
)

type fakeRuntime struct {
	home runtimepkg.Home
	kind string
}

func (r fakeRuntime) Kind() string { return r.kind }
func (r fakeRuntime) ComponentHome() runtimepkg.Home {
	return r.home
}
func (r fakeRuntime) RuntimeComponentHomePath() string {
	return r.home.Path
}
func (r fakeRuntime) RuntimeWorkspacePath(workspacePath string) string {
	return workspacePath
}
func (r fakeRuntime) Refresh(ctx context.Context, workspacePath string, threadID modeluuid.UUID) error {
	_, _, _ = ctx, workspacePath, threadID
	return fmt.Errorf("not implemented")
}
func (r fakeRuntime) Start(ctx context.Context, workspacePath string, threadID modeluuid.UUID) (runtimepkg.Status, error) {
	_, _, _ = ctx, workspacePath, threadID
	return runtimepkg.Status{}, fmt.Errorf("not implemented")
}
func (r fakeRuntime) Stop(ctx context.Context, workspacePath string, threadID modeluuid.UUID) error {
	_, _, _ = ctx, workspacePath, threadID
	return fmt.Errorf("not implemented")
}
func (r fakeRuntime) Interrupt(ctx context.Context, workspacePath string, threadID modeluuid.UUID) (bool, error) {
	_, _, _ = ctx, workspacePath, threadID
	return false, fmt.Errorf("not implemented")
}
func (r fakeRuntime) Status(ctx context.Context, workspacePath string, threadID modeluuid.UUID) (runtimepkg.Status, error) {
	_, _, _ = ctx, workspacePath, threadID
	return runtimepkg.Status{}, fmt.Errorf("not implemented")
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
func (f fakeFactory) ComponentHome(registration coremodel.Component) runtimepkg.Home {
	hostPath := registration.HomePath
	if hostPath == "" {
		hostPath = filepath.Join(f.componentsRoot, registration.Type, registration.Name)
	}
	return runtimepkg.Home{Path: hostPath}
}
func (f fakeFactory) RuntimeComponentHomePath(registration coremodel.Component, home runtimepkg.Home) string {
	_, _ = registration, home
	return home.Path
}
func (f fakeFactory) RuntimeWorkspacePath(workspacePath string) string {
	return workspacePath
}
func (f fakeFactory) Bind(registration coremodel.Component, home runtimepkg.Home, config runtimepkg.BindConfig) runtimepkg.Runtime {
	_, _, _ = registration, home, config
	return fakeRuntime{
		home: home,
		kind: f.kind,
	}
}

type fakeMessengerRecorder struct {
	payloads []message.OutboundPayload
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
func (c *fakeMessenger) Send(ctx context.Context, payload message.OutboundPayload) error {
	_ = ctx
	c.recorder.payloads = append(c.recorder.payloads, payload)
	return nil
}
func (c *fakeMessenger) StartChatAction(ctx context.Context, target message.ChatTarget, action message.ChatAction) (func(), error) {
	_, _, _ = ctx, target, action
	return func() {}, nil
}

type fakeAgentRecorder struct {
	prompts     []string
	homes       []runtimepkg.Home
	streamText  string
	finalText   string
	entered     chan struct{}
	release     <-chan struct{}
	interrupted chan struct{}
}

type fakeAgent struct {
	componentID modeluuid.UUID
	recorder    *fakeAgentRecorder
}

func (c *fakeAgent) Type() string { return "codex" }
func (c *fakeAgent) HandleTurn(ctx context.Context, turn component.Turn) (*component.TurnResult, error) {
	_ = ctx
	c.recorder.prompts = append(c.recorder.prompts, turn.Inbound.Text)
	if c.recorder.entered != nil {
		c.recorder.entered <- struct{}{}
	}
	if c.recorder.release != nil {
		<-c.recorder.release
	}
	if home, ok := turn.Runtime.ComponentHome(c.componentID); ok {
		c.recorder.homes = append(c.recorder.homes, home)
	}
	streamText := strings.TrimSpace(c.recorder.streamText)
	if streamText == "" {
		streamText = "working..."
	}
	if err := turn.Runtime.Send(context.Background(), message.OutboundPayload{
		Text: message.TextMessage{Text: streamText},
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

type fakeInterruptCommand struct{}

func (c *fakeAgent) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		{
			Pattern: "interrupt",
			Help:    "Interrupt the active fake turn",
			Build: func(req *clir.Request) (any, error) {
				_ = req
				return fakeInterruptCommand{}, nil
			},
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser),
		},
	}
}

func (c *fakeAgent) UsesLocalCommandRoutes() bool { return true }

func (c *fakeAgent) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	return commandengine.RegisterPattern[fakeInterruptCommand](registry, "interrupt", func(ctx context.Context, req commandengine.Request, _ fakeInterruptCommand) (commandengine.Result, error) {
		_, _ = ctx, req
		if c.recorder != nil && c.recorder.interrupted != nil {
			select {
			case c.recorder.interrupted <- struct{}{}:
			default:
			}
		}
		return commandengine.Result{Text: "interrupt requested"}, nil
	})
}

func newTestSystem(t *testing.T, root string, storage repository.Storage, recorder *fakeMessengerRecorder, agentRecorder *fakeAgentRecorder, events []component.InboundEvent, extras ...func(*component.Registry) error) *systempkg.System {
	t.Helper()

	registry := component.NewRegistry()
	if err := registry.Add("telegram", func(ctx context.Context, registration coremodel.Component, rt runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		_, _, _, _, _ = ctx, rt, home, storage, registration
		return &fakeMessenger{componentID: registration.ID, recorder: recorder, events: append([]component.InboundEvent(nil), events...)}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add("codex", func(ctx context.Context, registration coremodel.Component, rt runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
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

	system := systempkg.New(
		storage,
		map[string]systempkg.Workspace{},
		map[string]runtimepkg.Factory{"local": fakeFactory{kind: "local", rootDir: root, componentsRoot: filepath.Join(root, ".ctgbot", "components")}},
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
		Payload: message.InboundPayload{
			ProviderType:      "telegram",
			ProviderChatID:    "chat-1",
			ProviderThreadID:  "thread-7",
			ProviderMessageID: "msg-1",
			Actor: message.Actor{
				ID:    "bart",
				Label: "bart",
				Roles: []simplerbac.Role{simplerbac.RoleUser},
			},
			Text: message.TextMessage{Text: "hello"},
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

func TestHandleInboundSerializesTurnsPerThread(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	messengerRecorder := &fakeMessengerRecorder{}
	release := make(chan struct{})
	agentRecorder := &fakeAgentRecorder{
		entered: make(chan struct{}, 2),
		release: release,
	}
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

	inbound := func(id string, text string) component.InboundEvent {
		return component.InboundEvent{
			ComponentID: telegram.ID,
			ExternalID:  id,
			Payload: message.InboundPayload{
				ProviderType:      "telegram",
				ProviderChatID:    "chat-1",
				ProviderThreadID:  "thread-7",
				ProviderMessageID: id,
				Actor: message.Actor{
					ID:    "bart",
					Label: "bart",
					Roles: []simplerbac.Role{simplerbac.RoleUser},
				},
				Text: message.TextMessage{Text: text},
			},
		}
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := b.HandleInbound(context.Background(), inbound("msg-1", "first"))
		firstDone <- err
	}()
	select {
	case <-agentRecorder.entered:
	case <-time.After(time.Second):
		t.Fatal("first turn did not enter agent")
	}

	secondDone := make(chan error, 1)
	go func() {
		_, err := b.HandleInbound(context.Background(), inbound("msg-2", "second"))
		secondDone <- err
	}()

	select {
	case <-agentRecorder.entered:
		t.Fatal("second turn entered before first was released")
	case <-time.After(30 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first HandleInbound() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first HandleInbound() did not complete")
	}
	select {
	case <-agentRecorder.entered:
	case <-time.After(time.Second):
		t.Fatal("second turn did not enter agent after release")
	}
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("second HandleInbound() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("second HandleInbound() did not complete")
	}

	if got, want := len(agentRecorder.prompts), 2; got != want {
		t.Fatalf("agent prompts = %d, want %d", got, want)
	}
	if agentRecorder.prompts[0] != "first" || agentRecorder.prompts[1] != "second" {
		t.Fatalf("agent prompts = %#v, want [first second]", agentRecorder.prompts)
	}
}

func TestHandleInboundInterruptCommandBypassesTurnGate(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	messengerRecorder := &fakeMessengerRecorder{}
	release := make(chan struct{})
	agentRecorder := &fakeAgentRecorder{
		entered:     make(chan struct{}, 1),
		release:     release,
		interrupted: make(chan struct{}, 1),
	}
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

	inbound := func(id string, text string) component.InboundEvent {
		return component.InboundEvent{
			ComponentID: telegram.ID,
			ExternalID:  id,
			Payload: message.InboundPayload{
				ProviderType:      "telegram",
				ProviderChatID:    "chat-1",
				ProviderThreadID:  "thread-7",
				ProviderMessageID: id,
				Actor: message.Actor{
					ID:    "bart",
					Label: "bart",
					Roles: []simplerbac.Role{simplerbac.RoleUser},
				},
				Text: message.TextMessage{Text: text},
			},
		}
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := b.HandleInbound(context.Background(), inbound("msg-1", "first"))
		firstDone <- err
	}()
	select {
	case <-agentRecorder.entered:
	case <-time.After(time.Second):
		t.Fatal("first turn did not enter agent")
	}

	interruptDone := make(chan error, 1)
	go func() {
		_, err := b.HandleInbound(context.Background(), inbound("msg-2", "/codex interrupt"))
		interruptDone <- err
	}()

	select {
	case <-agentRecorder.interrupted:
	case <-time.After(time.Second):
		t.Fatal("interrupt command did not execute while first turn was active")
	}

	select {
	case err := <-interruptDone:
		if err != nil {
			t.Fatalf("interrupt HandleInbound() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interrupt HandleInbound() did not complete")
	}

	close(release)

	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first HandleInbound() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first HandleInbound() did not complete")
	}
}

func TestMessagingSendMessageRunsTargetThread(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	messengerRecorder := &fakeMessengerRecorder{}
	agentRecorder := &fakeAgentRecorder{finalText: "ack"}
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
		{ChatID: chat.ID, ComponentID: telegram.ID, Role: coremodel.ChatComponentRoleRelay, ExternalChatID: "chat-1", Enabled: true},
		{ChatID: chat.ID, ComponentID: codex.ID, Role: coremodel.ChatComponentRoleAgent, Enabled: true},
	} {
		binding := binding
		if err := storage.ChatComponents().Save(context.Background(), &binding); err != nil {
			t.Fatal(err)
		}
	}
	thread := &coremodel.Thread{ChatID: chat.ID, Label: "alpha"}
	if err := storage.Threads().Save(context.Background(), thread); err != nil {
		t.Fatal(err)
	}

	actor := coremodel.Actor{ID: "thread:source", Label: "source thread"}
	result, err := b.HandleResolvedInbound(context.Background(), component.ResolvedInbound{
		Chat:   *chat,
		Thread: *thread,
		Payload: message.InboundPayload{
			ProviderType: "thread",
			Text:         message.TextMessage{Text: "hello from another thread"},
			Actor:        actor,
		},
		Metadata: []string{"source_thread_id=11111111-2222-3333-4444-555555555555"},
	})
	if err != nil {
		t.Fatalf("HandleResolvedInbound() error = %v", err)
	}
	if result.Inbound == nil {
		t.Fatal("HandleResolvedInbound() inbound = nil")
	}
	if got := strings.TrimSpace(result.Inbound.Text); got != "hello from another thread" {
		t.Fatalf("result message text = %q, want %q", got, "hello from another thread")
	}

	messages, err := storage.Messages().ListByThreadID(context.Background(), thread.ID)
	if err != nil {
		t.Fatalf("ListByThreadID() error = %v", err)
	}
	if got, want := len(messages), 3; got != want {
		t.Fatalf("thread message count = %d, want %d", got, want)
	}
	var sawInbound, sawAck bool
	for _, stored := range messages {
		switch strings.TrimSpace(stored.Text) {
		case "hello from another thread":
			if stored.Direction == coremodel.MessageDirectionInbound {
				sawInbound = true
			}
		case "ack":
			if stored.Direction == coremodel.MessageDirectionOutbound {
				sawAck = true
			}
		}
	}
	if !sawInbound {
		t.Fatalf("did not find inbound message %q", "hello from another thread")
	}
	if !sawAck {
		t.Fatalf("did not find final outbound message %q", "ack")
	}
	if got, want := len(agentRecorder.prompts), 1; got != want {
		t.Fatalf("agent prompts = %d, want %d", got, want)
	}
	prompt := agentRecorder.prompts[0]
	for _, want := range []string{
		"[Internal thread message]",
		"From: source thread",
		"Reply path: hostbridge thread 11111111-2222-3333-4444-555555555555 message send <message>",
		"hello from another thread",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("agent prompt = %q, want to contain %q", prompt, want)
		}
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
		Payload: message.InboundPayload{
			ProviderType:      "telegram",
			ProviderChatID:    "chat-1",
			ProviderThreadID:  "thread-7",
			ProviderMessageID: "msg-1",
			Actor: message.Actor{
				ID:    "bart",
				Label: "bart",
				Roles: []simplerbac.Role{simplerbac.RoleUser},
			},
			Text: message.TextMessage{Text: "hello"},
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
			return registry.Add("tools", func(ctx context.Context, registration coremodel.Component, rt runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
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
		Payload: message.InboundPayload{
			ProviderType:      "telegram",
			ProviderChatID:    "chat-1",
			ProviderThreadID:  "thread-7",
			ProviderMessageID: "msg-1",
			Actor: message.Actor{
				ID:    "bart",
				Label: "bart",
				Roles: []simplerbac.Role{simplerbac.RoleUser},
			},
			Text: message.TextMessage{Text: "/tools ping"},
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
			return registry.Add(processcomponent.Type, func(ctx context.Context, registration coremodel.Component, rt runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
				_, _, _, _, _ = ctx, registration, rt, home, storage
				return processcomponent.New(&fakeProcessActions{}), nil
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
	process := &coremodel.Component{Type: processcomponent.Type, Name: processcomponent.Type, Runtime: "local", Enabled: true, IsDefault: true}
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
			Payload: message.InboundPayload{
				ProviderType:      "telegram",
				ProviderChatID:    "chat-1",
				ProviderThreadID:  "thread-7",
				ProviderMessageID: "msg-" + strings.ReplaceAll(text, " ", "-"),
				Actor: message.Actor{
					ID:    "bart",
					Label: "bart",
					Roles: []simplerbac.Role{simplerbac.RoleUser},
				},
				Text: message.TextMessage{Text: text},
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

func TestHandleInboundDropsUnknownChatAndRecordsDrop(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	messengerRecorder := &fakeMessengerRecorder{}
	agentRecorder := &fakeAgentRecorder{}
	system := newTestSystem(t, root, storage, messengerRecorder, agentRecorder, nil)
	var logs []string
	b := broker.New(storage, system, func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	})

	telegram := &coremodel.Component{Type: "telegram", Name: "telegram", Runtime: "local", Enabled: true, IsDefault: true}
	if err := storage.Components().Save(context.Background(), telegram); err != nil {
		t.Fatal(err)
	}

	outcome, err := b.HandleInbound(context.Background(), component.InboundEvent{
		ComponentID: telegram.ID,
		ExternalID:  "msg-unknown",
		Payload: message.InboundPayload{
			ProviderType:      "telegram",
			ProviderChatID:    "chat-new",
			ProviderThreadID:  "thread-new",
			ProviderMessageID: "msg-unknown",
			ChatLabel:         "New chat",
			Actor: message.Actor{
				ID:    "bart",
				Label: "Bart",
				Roles: []simplerbac.Role{simplerbac.RoleUser},
			},
			Text: message.TextMessage{Text: "hello from new chat"},
		},
	})
	if err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	if !outcome.Dropped {
		t.Fatal("expected unknown inbound chat to be dropped")
	}
	drops, err := storage.InboundDrops().List(context.Background())
	if err != nil {
		t.Fatalf("InboundDrops().List() error = %v", err)
	}
	if len(drops) != 1 {
		t.Fatalf("drop count = %d, want 1", len(drops))
	}
	drop := drops[0]
	if got, want := drop.ExternalChatID, "chat-new"; got != want {
		t.Fatalf("ExternalChatID = %q, want %q", got, want)
	}
	if got, want := drop.MessageCount, int64(1); got != want {
		t.Fatalf("MessageCount = %d, want %d", got, want)
	}
	if got, want := drop.LastTextPreview, "hello from new chat"; got != want {
		t.Fatalf("LastTextPreview = %q, want %q", got, want)
	}
	if got := len(messengerRecorder.payloads); got != 0 {
		t.Fatalf("relay payloads = %d, want 0", got)
	}
	if got := len(logs); got == 0 {
		t.Fatal("expected drop log")
	}
	if logLine := logs[len(logs)-1]; !strings.Contains(logLine, `reason=no-source-binding`) || !strings.Contains(logLine, `external_chat="chat-new"`) || !strings.Contains(logLine, `preview="hello from new chat"`) {
		t.Fatalf("drop log = %q", logLine)
	}
}

func TestHandleInboundInitReplyGuidesUnknownChatActivation(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	messengerRecorder := &fakeMessengerRecorder{}
	agentRecorder := &fakeAgentRecorder{}
	system := newTestSystem(t, root, storage, messengerRecorder, agentRecorder, nil)
	b := broker.New(storage, system, nil)

	telegram := &coremodel.Component{Type: "telegram", Name: "telegram", Runtime: "local", Enabled: true, IsDefault: true}
	if err := storage.Components().Save(context.Background(), telegram); err != nil {
		t.Fatal(err)
	}

	outcome, err := b.HandleInbound(context.Background(), component.InboundEvent{
		ComponentID: telegram.ID,
		ExternalID:  "msg-init",
		Payload: message.InboundPayload{
			ProviderType:      "telegram",
			ProviderChatID:    "chat-new",
			ProviderThreadID:  "thread-new",
			ProviderMessageID: "msg-init",
			ChatLabel:         "New chat",
			Actor: message.Actor{
				ID:    "bart",
				Label: "Bart",
				Roles: []simplerbac.Role{simplerbac.RoleUser},
			},
			Text: message.TextMessage{Text: "/init"},
		},
	})
	if err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	if !outcome.Dropped {
		t.Fatal("expected /init from unknown chat to be dropped")
	}
	if got, want := len(messengerRecorder.payloads), 1; got != want {
		t.Fatalf("relay payloads = %d, want %d", got, want)
	}
	reply := messengerRecorder.payloads[0].Text.Text
	for _, want := range []string{
		"chat is not bound",
		"component: telegram",
		"external_chat_id: chat-new",
		`ctgbot chat bind telegram chat-new "New chat"`,
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("init reply missing %q:\n%s", want, reply)
		}
	}
}

func TestHandleInboundInitReplyGuidesDisabledChatEnable(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	messengerRecorder := &fakeMessengerRecorder{}
	agentRecorder := &fakeAgentRecorder{}
	system := newTestSystem(t, root, storage, messengerRecorder, agentRecorder, nil)
	b := broker.New(storage, system, nil)

	chat := &coremodel.Chat{Label: "team", Enabled: false}
	if err := storage.Chats().Save(context.Background(), chat); err != nil {
		t.Fatal(err)
	}
	telegram := &coremodel.Component{Type: "telegram", Name: "telegram", Runtime: "local", Enabled: true, IsDefault: true}
	if err := storage.Components().Save(context.Background(), telegram); err != nil {
		t.Fatal(err)
	}
	for _, binding := range []coremodel.ChatComponent{
		{ChatID: chat.ID, ComponentID: telegram.ID, Role: coremodel.ChatComponentRoleSource, ExternalChatID: "chat-1", Enabled: true},
		{ChatID: chat.ID, ComponentID: telegram.ID, Role: coremodel.ChatComponentRoleRelay, ExternalChatID: "chat-1", Enabled: true},
	} {
		binding := binding
		if err := storage.ChatComponents().Save(context.Background(), &binding); err != nil {
			t.Fatal(err)
		}
	}

	outcome, err := b.HandleInbound(context.Background(), component.InboundEvent{
		ComponentID: telegram.ID,
		ExternalID:  "msg-disabled",
		Payload: message.InboundPayload{
			ProviderType:      "telegram",
			ProviderChatID:    "chat-1",
			ProviderThreadID:  "thread-1",
			ProviderMessageID: "msg-disabled",
			Text:              message.TextMessage{Text: "/init"},
		},
	})
	if err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	if !outcome.Dropped {
		t.Fatal("expected /init from disabled chat to be dropped")
	}
	if got, want := len(messengerRecorder.payloads), 1; got != want {
		t.Fatalf("relay payloads = %d, want %d", got, want)
	}
	reply := messengerRecorder.payloads[0].Text.Text
	want := "ctgbot config chat " + chat.ID.String() + " set chat.enabled true"
	if !strings.Contains(reply, want) {
		t.Fatalf("init reply missing %q:\n%s", want, reply)
	}
	drops, err := storage.InboundDrops().List(context.Background())
	if err != nil {
		t.Fatalf("InboundDrops().List() error = %v", err)
	}
	if len(drops) != 0 {
		t.Fatalf("drop count = %d, want 0 for disabled known chat", len(drops))
	}
}

func TestRunStartsEnabledInboundSources(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	messengerRecorder := &fakeMessengerRecorder{}
	agentRecorder := &fakeAgentRecorder{}
	system := newTestSystem(t, root, storage, messengerRecorder, agentRecorder, []component.InboundEvent{{
		ExternalID: "msg-2",
		Payload: message.InboundPayload{
			ProviderType:      "telegram",
			ProviderChatID:    "chat-2",
			ProviderThreadID:  "thread-9",
			ProviderMessageID: "msg-2",
			Actor: message.Actor{
				ID:    "bart",
				Label: "bart",
				Roles: []simplerbac.Role{simplerbac.RoleUser},
			},
			Text: message.TextMessage{Text: "ping"},
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
		Pattern: "tools ping",
		Help:    "Reply with pong",
		Build: func(req *clir.Request) (any, error) {
			_ = req
			return pingCommand{}, nil
		},
		Sources: []commandengine.Source{commandengine.SourceMessage},
		Policy:  simplerbac.Public(),
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

func (f *fakeProcessActions) Install(ctx context.Context) error {
	_ = ctx
	return nil
}

func (f *fakeProcessActions) Upgrade(ctx context.Context) error {
	_ = ctx
	return nil
}

func (f *fakeProcessActions) Quit(ctx context.Context) error {
	_ = ctx
	return nil
}
