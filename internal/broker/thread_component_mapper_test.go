package broker_test

import (
	"context"
	"testing"

	broker "github.com/bartdeboer/ctgbot/internal/broker"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

func TestThreadComponentMapperScopesInboundThreadIDsPerChat(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	mapper := broker.NewThreadComponentMapper(storage)

	chatA := &coremodel.Chat{Label: "alpha", Enabled: true}
	chatB := &coremodel.Chat{Label: "beta", Enabled: true}
	component := &coremodel.Component{Type: "telegram", Name: "telegram", Enabled: true, IsDefault: true}
	for _, value := range []any{chatA, chatB, component} {
		switch value := value.(type) {
		case *coremodel.Chat:
			if err := storage.Chats().Save(ctx, value); err != nil {
				t.Fatal(err)
			}
		case *coremodel.Component:
			if err := storage.Components().Save(ctx, value); err != nil {
				t.Fatal(err)
			}
		}
	}

	bindingA := coremodel.ChatComponent{
		ID:             modeluuid.New(),
		ChatID:         chatA.ID,
		ComponentID:    component.ID,
		Role:           coremodel.ChatComponentRoleSource,
		ExternalChatID: "chat-a",
		Enabled:        true,
	}
	bindingB := coremodel.ChatComponent{
		ID:             modeluuid.New(),
		ChatID:         chatB.ID,
		ComponentID:    component.ID,
		Role:           coremodel.ChatComponentRoleSource,
		ExternalChatID: "chat-b",
		Enabled:        true,
	}

	threadA, err := mapper.EnsureThread(ctx, bindingA, "")
	if err != nil {
		t.Fatalf("EnsureThread(alpha) error = %v", err)
	}
	threadB, err := mapper.EnsureThread(ctx, bindingB, "")
	if err != nil {
		t.Fatalf("EnsureThread(beta) error = %v", err)
	}
	if threadA.ID == threadB.ID {
		t.Fatalf("expected distinct threads, got %s", threadA.ID)
	}
	if threadA.ChatID != chatA.ID || threadB.ChatID != chatB.ID {
		t.Fatalf("unexpected chat mapping: alpha=%s beta=%s", threadA.ChatID, threadB.ChatID)
	}

	threadAAgain, err := mapper.EnsureThread(ctx, bindingA, "")
	if err != nil {
		t.Fatalf("EnsureThread(alpha again) error = %v", err)
	}
	threadBAgain, err := mapper.EnsureThread(ctx, bindingB, "")
	if err != nil {
		t.Fatalf("EnsureThread(beta again) error = %v", err)
	}
	if threadAAgain.ID != threadA.ID || threadBAgain.ID != threadB.ID {
		t.Fatalf("expected stable thread mappings, got alpha=%s/%s beta=%s/%s", threadA.ID, threadAAgain.ID, threadB.ID, threadBAgain.ID)
	}

	componentThreadID, ok, err := mapper.ComponentThreadID(ctx, threadA.ID, component.ID)
	if err != nil {
		t.Fatalf("ComponentThreadID(alpha) error = %v", err)
	}
	if !ok || componentThreadID != "default" {
		t.Fatalf("ComponentThreadID(alpha) = (%q, %t), want (default, true)", componentThreadID, ok)
	}

	target, ok, err := mapper.RelayTarget(ctx, threadA.ID, coremodel.ChatComponent{
		ChatID:         chatA.ID,
		ComponentID:    component.ID,
		Role:           coremodel.ChatComponentRoleRelay,
		ExternalChatID: "chat-a",
		Enabled:        true,
	})
	if err != nil {
		t.Fatalf("RelayTarget(alpha) error = %v", err)
	}
	if !ok || target.ProviderChatID != "chat-a" || target.ProviderThreadID != "default" {
		t.Fatalf("RelayTarget(alpha) = %#v, ok=%t", target, ok)
	}
}

func TestThreadComponentMapperRelayTargetFallsBackToExternalChatID(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	mapper := broker.NewThreadComponentMapper(storage)

	chat := &coremodel.Chat{Label: "team", Enabled: true}
	thread := &coremodel.Thread{ChatID: modeluuid.New()}
	component := &coremodel.Component{Type: "telegram", Name: "relay", Enabled: true}

	if err := storage.Chats().Save(ctx, chat); err != nil {
		t.Fatal(err)
	}
	thread.ChatID = chat.ID
	if err := storage.Threads().Save(ctx, thread); err != nil {
		t.Fatal(err)
	}
	if err := storage.Components().Save(ctx, component); err != nil {
		t.Fatal(err)
	}

	target, ok, err := mapper.RelayTarget(ctx, thread.ID, coremodel.ChatComponent{
		ChatID:         chat.ID,
		ComponentID:    component.ID,
		Role:           coremodel.ChatComponentRoleRelay,
		ExternalChatID: "telegram-chat-1",
		Enabled:        true,
	})
	if err != nil {
		t.Fatalf("RelayTarget() error = %v", err)
	}
	if !ok {
		t.Fatal("RelayTarget() = not found, want fallback target")
	}
	if target.ProviderChatID != "telegram-chat-1" {
		t.Fatalf("ProviderChatID = %q, want telegram-chat-1", target.ProviderChatID)
	}
	if target.ProviderThreadID != "" {
		t.Fatalf("ProviderThreadID = %q, want empty", target.ProviderThreadID)
	}
}

func TestThreadComponentMapperReusesVisibleDefaultThreadForMailboxMapping(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	mapper := broker.NewThreadComponentMapper(storage)

	chat := &coremodel.Chat{Label: "mailbox", Enabled: true}
	telegram := &coremodel.Component{Type: "telegram", Name: "telegram", Enabled: true}
	gmail := &coremodel.Component{Type: "gmail", Name: "work", Enabled: true}
	for _, registration := range []*coremodel.Component{telegram, gmail} {
		if err := storage.Components().Save(ctx, registration); err != nil {
			t.Fatal(err)
		}
	}
	if err := storage.Chats().Save(ctx, chat); err != nil {
		t.Fatal(err)
	}

	visibleThread := &coremodel.Thread{ChatID: chat.ID, Label: "visible"}
	if err := storage.Threads().Save(ctx, visibleThread); err != nil {
		t.Fatal(err)
	}
	if err := mapper.BindComponentThreadID(ctx, visibleThread.ID, telegram.ID, "0"); err != nil {
		t.Fatalf("BindComponentThreadID(telegram) error = %v", err)
	}

	gmailBinding := coremodel.ChatComponent{
		ChatID:         chat.ID,
		ComponentID:    gmail.ID,
		Role:           coremodel.ChatComponentRoleSource,
		ExternalChatID: "bart@example.com",
		Enabled:        true,
	}
	thread, err := mapper.EnsureThread(ctx, gmailBinding, "bart@example.com")
	if err != nil {
		t.Fatalf("EnsureThread(gmail) error = %v", err)
	}
	if thread.ID != visibleThread.ID {
		t.Fatalf("EnsureThread(gmail) thread = %s, want visible thread %s", thread.ID, visibleThread.ID)
	}

	componentThreadID, ok, err := mapper.ComponentThreadID(ctx, visibleThread.ID, gmail.ID)
	if err != nil {
		t.Fatalf("ComponentThreadID(gmail) error = %v", err)
	}
	if !ok || componentThreadID != "bart@example.com" {
		t.Fatalf("ComponentThreadID(gmail) = (%q, %t), want mailbox mapping", componentThreadID, ok)
	}
}

func TestThreadComponentMapperDoesNotReuseVisibleDefaultThreadForNonMailboxMapping(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	mapper := broker.NewThreadComponentMapper(storage)

	chat := &coremodel.Chat{Label: "chat", Enabled: true}
	telegram := &coremodel.Component{Type: "telegram", Name: "telegram", Enabled: true}
	source := &coremodel.Component{Type: "other", Name: "source", Enabled: true}
	for _, registration := range []*coremodel.Component{telegram, source} {
		if err := storage.Components().Save(ctx, registration); err != nil {
			t.Fatal(err)
		}
	}
	if err := storage.Chats().Save(ctx, chat); err != nil {
		t.Fatal(err)
	}

	visibleThread := &coremodel.Thread{ChatID: chat.ID, Label: "visible"}
	if err := storage.Threads().Save(ctx, visibleThread); err != nil {
		t.Fatal(err)
	}
	if err := mapper.BindComponentThreadID(ctx, visibleThread.ID, telegram.ID, "0"); err != nil {
		t.Fatalf("BindComponentThreadID(telegram) error = %v", err)
	}

	sourceBinding := coremodel.ChatComponent{
		ChatID:         chat.ID,
		ComponentID:    source.ID,
		Role:           coremodel.ChatComponentRoleSource,
		ExternalChatID: "chat-1",
		Enabled:        true,
	}
	thread, err := mapper.EnsureThread(ctx, sourceBinding, "api-thread-1")
	if err != nil {
		t.Fatalf("EnsureThread(source) error = %v", err)
	}
	if thread.ID == visibleThread.ID {
		t.Fatalf("EnsureThread(source) reused visible thread for non-mailbox mapping %s", visibleThread.ID)
	}
}
