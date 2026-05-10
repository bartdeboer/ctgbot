package messagingcomponent

import (
	"context"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	messagingdomain "github.com/bartdeboer/ctgbot/internal/messaging"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func TestStatusShowsCurrentThread(t *testing.T) {
	ctx := context.Background()
	storage, thread := testMessagingStorage(t, ctx)
	engine := testMessagingEngine(t, storage)

	result, err := engine.Run(ctx, testMessagingRequest(thread.ID), []string{"status"})
	if err != nil {
		t.Fatalf("Run(status) error = %v", err)
	}
	for _, want := range []string{
		"thread status",
		"chat_label: Codex #1",
		"thread_label: ctgbot 2",
		"- telegram source external_chat_id=-100 external_thread_id=845",
	} {
		if !strings.Contains(result.Text, want) {
			t.Fatalf("status missing %q:\n%s", want, result.Text)
		}
	}
}

func TestThreadLabelSetUpdatesCurrentThread(t *testing.T) {
	ctx := context.Background()
	storage, thread := testMessagingStorage(t, ctx)
	engine := testMessagingEngine(t, storage)

	result, err := engine.Run(ctx, testMessagingRequest(thread.ID), []string{"thread", "label", "set", "new", "label"})
	if err != nil {
		t.Fatalf("Run(thread label set) error = %v", err)
	}
	if got, want := strings.TrimSpace(result.Text), "thread label set: new label"; got != want {
		t.Fatalf("result = %q, want %q", got, want)
	}
	updated, err := storage.Threads().GetByID(ctx, thread.ID)
	if err != nil {
		t.Fatalf("GetByID(thread) error = %v", err)
	}
	if got, want := updated.Label, "new label"; got != want {
		t.Fatalf("thread label = %q, want %q", got, want)
	}
}

func testMessagingStorage(t *testing.T, ctx context.Context) (*repository.MemoryStorage, coremodel.Thread) {
	t.Helper()
	storage := repository.NewMemory()
	chat := &coremodel.Chat{Label: "Codex #1", Enabled: true}
	if err := storage.Chats().Save(ctx, chat); err != nil {
		t.Fatalf("Save(chat) error = %v", err)
	}
	thread := &coremodel.Thread{ChatID: chat.ID, Label: "ctgbot 2"}
	if err := storage.Threads().Save(ctx, thread); err != nil {
		t.Fatalf("Save(thread) error = %v", err)
	}
	telegram := &coremodel.Component{Type: "telegram", Name: "telegram", Enabled: true}
	if err := storage.Components().Save(ctx, telegram); err != nil {
		t.Fatalf("Save(component) error = %v", err)
	}
	binding := &coremodel.ChatComponent{
		ChatID:         chat.ID,
		ComponentID:    telegram.ID,
		Role:           coremodel.ChatComponentRoleSource,
		ExternalChatID: "-100",
		Enabled:        true,
	}
	if err := storage.ChatComponents().Save(ctx, binding); err != nil {
		t.Fatalf("Save(chat component) error = %v", err)
	}
	mapping := &coremodel.ThreadComponentMapping{
		ThreadID:          thread.ID,
		ChatID:            chat.ID,
		ComponentID:       telegram.ID,
		ComponentThreadID: "845",
	}
	if err := storage.ThreadComponentMappings().Save(ctx, mapping); err != nil {
		t.Fatalf("Save(thread mapping) error = %v", err)
	}
	return storage, *thread
}

func testMessagingEngine(t *testing.T, storage repository.Storage) *commandengine.Engine {
	t.Helper()
	engine, err := commandset.NewEngineForSource(commandengine.SourceMessage, New(messagingdomain.New(storage), nil))
	if err != nil {
		t.Fatalf("NewEngineForSource() error = %v", err)
	}
	return engine
}

func testMessagingRequest(threadID modeluuid.UUID) commandengine.Request {
	return commandengine.Request{Context: commandengine.Context{
		ThreadID: threadID,
		Actor: commandengine.Actor{
			ID:    "bart",
			Label: "Bart",
			Roles: []simplerbac.Role{simplerbac.RoleRoot},
		},
	}}
}
