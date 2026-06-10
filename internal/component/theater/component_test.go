package theater

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	messagingdomain "github.com/bartdeboer/ctgbot/internal/messaging"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func TestTheaterSubscribeReadFlow(t *testing.T) {
	ctx := context.Background()
	component := newTestComponent(t)
	engine := newTestEngine(t, component)
	viewer := createTestThread(t, ctx, component.storage, "Codex #1", "ctgbot 2")
	target := createTestThread(t, ctx, component.storage, "Theaters", "qwen-parser-lab")
	base := testRequest(viewer.ID)

	appendTestMessage(t, ctx, component.storage, target, "Claude #1", "Parser image completed.", time.Date(2026, 6, 9, 9, 0, 0, 0, time.UTC))
	appendTestMessage(t, ctx, component.storage, target, "qwen 1", "Smoke task passed.", time.Date(2026, 6, 9, 9, 13, 0, 0, time.UTC))

	result, err := engine.Run(ctx, base, []string{Type, target.ID.String(), "subscribe"})
	if err != nil {
		t.Fatalf("subscribe error = %v", err)
	}
	if result.Text != "subscribed: qwen-parser-lab" {
		t.Fatalf("subscribe = %q", result.Text)
	}

	updates, err := component.NewUpdates(ctx, componentUpdateRequest(viewer.ID))
	if err != nil {
		t.Fatalf("NewUpdates() error = %v", err)
	}
	if len(updates) != 1 || updates[0].Label != "qwen-parser-lab" || updates[0].Count != 2 {
		t.Fatalf("updates = %#v", updates)
	}

	status, err := engine.Run(ctx, base, []string{Type, target.ID.String(), "status"})
	if err != nil {
		t.Fatalf("target status error = %v", err)
	}
	if !strings.Contains(status.Text, "unread messages: 2") {
		t.Fatalf("target status = %q", status.Text)
	}

	read, err := engine.Run(ctx, base, []string{Type, target.ID.String(), "read", "--limit", "1"})
	if err != nil {
		t.Fatalf("read error = %v", err)
	}
	if strings.Contains(read.Text, "Parser image completed.") || !strings.Contains(read.Text, "qwen 1: Smoke task passed.") {
		t.Fatalf("read = %q, want only latest message", read.Text)
	}

	updates, err = component.NewUpdates(ctx, componentUpdateRequest(viewer.ID))
	if err != nil {
		t.Fatalf("NewUpdates after read error = %v", err)
	}
	if len(updates) != 0 {
		t.Fatalf("updates after read = %#v, want none", updates)
	}

	result, err = engine.Run(ctx, base, []string{Type, target.ID.String(), "unsubscribe"})
	if err != nil {
		t.Fatalf("unsubscribe error = %v", err)
	}
	if result.Text != "unsubscribed: qwen-parser-lab" {
		t.Fatalf("unsubscribe = %q", result.Text)
	}
}

func TestTheaterStatusWithoutSubscriptions(t *testing.T) {
	ctx := context.Background()
	component := newTestComponent(t)
	engine := newTestEngine(t, component)
	viewer := createTestThread(t, ctx, component.storage, "Codex #1", "ctgbot 2")
	result, err := engine.Run(ctx, testRequest(viewer.ID), []string{Type, "status"})
	if err != nil {
		t.Fatalf("status error = %v", err)
	}
	if result.Text != "no theater subscriptions" {
		t.Fatalf("status = %q", result.Text)
	}
}

func newTestComponent(t *testing.T) *Component {
	t.Helper()
	store, err := openStore(t.TempDir())
	if err != nil {
		t.Fatalf("openStore() error = %v", err)
	}
	storage := repository.NewMemory()
	return &Component{registration: coremodel.Component{Type: Type, Name: Type}, store: store, storage: storage, messages: messagingdomain.New(storage)}
}

func newTestEngine(t *testing.T, c *Component) *commandengine.Engine {
	t.Helper()
	engine, err := commandset.NewBoundEngineForSource(commandengine.SourceMessage, []commandset.BoundSurface{{Surface: c, ComponentRef: Type, ComponentType: Type}})
	if err != nil {
		t.Fatalf("NewBoundEngineForSource() error = %v", err)
	}
	return engine
}

func testRequest(threadID modeluuid.UUID) commandengine.Request {
	return commandengine.Request{Context: commandengine.Context{Source: commandengine.SourceMessage, ThreadID: threadID, Actor: coremodel.Actor{ID: "tester", Label: "tester", Roles: []simplerbac.Role{simplerbac.RoleAgent}}}}
}

func componentUpdateRequest(threadID modeluuid.UUID) component.UpdateRequest {
	return component.UpdateRequest{ThreadID: threadID}
}

func createTestThread(t *testing.T, ctx context.Context, storage repository.Storage, chatLabel string, threadLabel string) coremodel.Thread {
	t.Helper()
	return createTestThreadWithID(t, ctx, storage, modeluuid.New(), chatLabel, modeluuid.New(), threadLabel)
}

func createTestThreadWithID(t *testing.T, ctx context.Context, storage repository.Storage, chatID modeluuid.UUID, chatLabel string, threadID modeluuid.UUID, threadLabel string) coremodel.Thread {
	t.Helper()
	chat := &coremodel.Chat{ID: chatID, Label: chatLabel, Enabled: true}
	if err := storage.Chats().Save(ctx, chat); err != nil {
		t.Fatalf("save chat: %v", err)
	}
	thread := &coremodel.Thread{ID: threadID, ChatID: chat.ID, Label: threadLabel}
	if err := storage.Threads().Save(ctx, thread); err != nil {
		t.Fatalf("save thread: %v", err)
	}
	return *thread
}

func appendTestMessage(t *testing.T, ctx context.Context, storage repository.Storage, thread coremodel.Thread, actorLabel string, text string, createdAt time.Time) {
	t.Helper()
	message := &coremodel.ThreadMessage{
		ID:         modeluuid.New(),
		ChatID:     thread.ChatID,
		ThreadID:   thread.ID,
		Direction:  coremodel.MessageDirectionInbound,
		Role:       coremodel.MessageRoleAgent,
		Kind:       coremodel.MessageKindMessage,
		ActorID:    actorLabel,
		ActorLabel: actorLabel,
		Text:       text,
		CreatedAt:  createdAt,
	}
	if err := storage.Messages().Append(ctx, message); err != nil {
		t.Fatalf("append message: %v", err)
	}
}

func TestTheaterHandleTurnRelaysInternalThreadPosts(t *testing.T) {
	component := newTestComponent(t)
	component.registration.ID = modeluuid.New()
	turn := componentTurn("thread", "Claude #1", "parser image ready")
	turn.Prompt = "[Internal thread message]\nFrom: Claude #1\n\nMessage: parser image ready"
	result, err := component.HandleTurn(context.Background(), turn)
	if err != nil {
		t.Fatalf("HandleTurn() error = %v", err)
	}
	if result == nil || result.Final == nil {
		t.Fatal("HandleTurn() returned no final message")
	}
	if got, want := result.Final.Text, "Claude #1: parser image ready"; got != want {
		t.Fatalf("final text = %q, want %q", got, want)
	}
	if result.Final.ComponentID != component.registration.ID {
		t.Fatalf("component id = %s, want %s", result.Final.ComponentID, component.registration.ID)
	}
	if got, want := result.Final.MetadataJSON, "relay=theater"; got != want {
		t.Fatalf("metadata = %q, want %q", got, want)
	}
}

func TestTheaterHandleTurnIgnoresExternalProviderPosts(t *testing.T) {
	component := newTestComponent(t)
	result, err := component.HandleTurn(context.Background(), componentTurn("telegram", "Bart", "hello"))
	if err != nil {
		t.Fatalf("HandleTurn() error = %v", err)
	}
	if result != nil {
		t.Fatalf("HandleTurn() = %#v, want nil", result)
	}
}

func TestTheaterReadHidesOwnRelayMessages(t *testing.T) {
	ctx := context.Background()
	component := newTestComponent(t)
	component.registration.ID = modeluuid.New()
	engine := newTestEngine(t, component)
	viewer := createTestThread(t, ctx, component.storage, "Codex #1", "ctgbot 2")
	target := createTestThread(t, ctx, component.storage, "Theaters", "qwen-parser-lab")
	appendTestMessage(t, ctx, component.storage, target, "Claude #1", "parser image ready", time.Date(2026, 6, 9, 9, 0, 0, 0, time.UTC))
	if err := component.storage.Messages().Append(ctx, &coremodel.ThreadMessage{
		ID:          modeluuid.New(),
		ChatID:      target.ChatID,
		ThreadID:    target.ID,
		Direction:   coremodel.MessageDirectionOutbound,
		Role:        coremodel.MessageRoleAgent,
		Kind:        coremodel.MessageKindMessage,
		ComponentID: component.registration.ID,
		ActorID:     Type,
		ActorLabel:  "theater",
		Text:        "Claude #1: parser image ready",
		CreatedAt:   time.Date(2026, 6, 9, 9, 0, 1, 0, time.UTC),
	}); err != nil {
		t.Fatalf("append relay message: %v", err)
	}
	if _, err := component.store.subscribe(ctx, viewer.ID, target.ID, target.Label); err != nil {
		t.Fatal(err)
	}
	updates, err := component.NewUpdates(ctx, componentUpdateRequest(viewer.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 1 || updates[0].Count != 1 {
		t.Fatalf("updates = %#v, want one visible message", updates)
	}
	read, err := engine.Run(ctx, testRequest(viewer.ID), []string{Type, target.ID.String(), "read"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(read.Text, "theater: Claude #1") || strings.Count(read.Text, "parser image ready") != 1 {
		t.Fatalf("read = %q, want only original inbound post", read.Text)
	}
}

func componentTurn(provider string, actorLabel string, text string) component.Turn {
	return component.Turn{Inbound: coremodel.ThreadMessage{
		Direction:    coremodel.MessageDirectionInbound,
		Role:         coremodel.MessageRoleUser,
		Kind:         coremodel.MessageKindMessage,
		ActorID:      actorLabel,
		ActorLabel:   actorLabel,
		Text:         text,
		MetadataJSON: "provider=" + provider,
	}}
}
