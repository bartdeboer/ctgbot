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
		ID:                modeluuid.New(),
		ChatID:            chatA.ID,
		ComponentID:       component.ID,
		Role:              coremodel.ChatComponentRoleSource,
		ExternalChannelID: "chat-a",
		Enabled:           true,
	}
	bindingB := coremodel.ChatComponent{
		ID:                modeluuid.New(),
		ChatID:            chatB.ID,
		ComponentID:       component.ID,
		Role:              coremodel.ChatComponentRoleSource,
		ExternalChannelID: "chat-b",
		Enabled:           true,
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
		ChatID:            chatA.ID,
		ComponentID:       component.ID,
		Role:              coremodel.ChatComponentRoleRelay,
		ExternalChannelID: "chat-a",
		Enabled:           true,
	})
	if err != nil {
		t.Fatalf("RelayTarget(alpha) error = %v", err)
	}
	if !ok || target.ProviderChannelID != "chat-a" || target.ProviderThreadID != "default" {
		t.Fatalf("RelayTarget(alpha) = %#v, ok=%t", target, ok)
	}
}

func TestThreadComponentMapperRelayTargetFallsBackToExternalChannelID(t *testing.T) {
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
		ChatID:            chat.ID,
		ComponentID:       component.ID,
		Role:              coremodel.ChatComponentRoleRelay,
		ExternalChannelID: "telegram-chat-1",
		Enabled:           true,
	})
	if err != nil {
		t.Fatalf("RelayTarget() error = %v", err)
	}
	if !ok {
		t.Fatal("RelayTarget() = not found, want fallback target")
	}
	if target.ProviderChannelID != "telegram-chat-1" {
		t.Fatalf("ProviderChannelID = %q, want telegram-chat-1", target.ProviderChannelID)
	}
	if target.ProviderThreadID != "" {
		t.Fatalf("ProviderThreadID = %q, want empty", target.ProviderThreadID)
	}
}

func TestThreadComponentMapperReusesVisibleDefaultThreadForSourceDefaultMapping(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	mapper := broker.NewThreadComponentMapper(storage)

	chat := &coremodel.Chat{Label: "source default", Enabled: true}
	telegram := &coremodel.Component{Type: "telegram", Name: "telegram", Enabled: true}
	source := &coremodel.Component{Type: "mailbox", Name: "personal", Enabled: true}
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
		ChatID:            chat.ID,
		ComponentID:       source.ID,
		Role:              coremodel.ChatComponentRoleSource,
		ExternalChannelID: "mailbox-personal",
		Enabled:           true,
	}
	thread, err := mapper.EnsureThread(ctx, sourceBinding, "mailbox-personal")
	if err != nil {
		t.Fatalf("EnsureThread(source) error = %v", err)
	}
	if thread.ID != visibleThread.ID {
		t.Fatalf("EnsureThread(source) thread = %s, want visible thread %s", thread.ID, visibleThread.ID)
	}

	componentThreadID, ok, err := mapper.ComponentThreadID(ctx, visibleThread.ID, source.ID)
	if err != nil {
		t.Fatalf("ComponentThreadID(source) error = %v", err)
	}
	if !ok || componentThreadID != "mailbox-personal" {
		t.Fatalf("ComponentThreadID(source) = (%q, %t), want source-default mapping", componentThreadID, ok)
	}
}

func TestThreadComponentMapperDoesNotReuseVisibleDefaultThreadForNonMatchingSourceThread(t *testing.T) {
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
		ChatID:            chat.ID,
		ComponentID:       source.ID,
		Role:              coremodel.ChatComponentRoleSource,
		ExternalChannelID: "chat-1",
		Enabled:           true,
	}
	thread, err := mapper.EnsureThread(ctx, sourceBinding, "api-thread-1")
	if err != nil {
		t.Fatalf("EnsureThread(source) error = %v", err)
	}
	if thread.ID == visibleThread.ID {
		t.Fatalf("EnsureThread(source) reused visible thread for non-matching source thread %s", visibleThread.ID)
	}
}
