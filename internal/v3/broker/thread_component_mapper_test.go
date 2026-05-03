package broker_test

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	v3broker "github.com/bartdeboer/ctgbot/internal/v3/broker"
	"github.com/bartdeboer/ctgbot/internal/v3/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v3/repository"
)

func TestThreadComponentMapperScopesInboundThreadIDsPerChat(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	mapper := v3broker.NewThreadComponentMapper(storage)

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
