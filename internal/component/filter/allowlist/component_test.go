package allowlist

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func TestAllowlistCommandsViewWhitelistListRemoveAndPolicy(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	allowlistComponent := New(storage)
	chat, sourceBinding := seedAllowlistScope(t, storage)
	eventJSON := mustJSON(t, component.InboundEvent{Payload: message.InboundPayload{
		ProviderMessageID: "message-1",
		Text:              message.TextMessage{Text: "Subject: Hello\n\nFull body line one.\nFull body line two."},
	}})
	drop := &coremodel.DroppedEvent{
		Status:            "pending",
		Action:            "drop",
		Reason:            "allowlist-unknown-sender",
		ChatID:            chat.ID,
		SourceBindingID:   sourceBinding.ID,
		ComponentID:       sourceBinding.ComponentID,
		ProviderChannelID: "inbox",
		ProviderThreadID:  "thread-1",
		ProviderMessageID: "message-1",
		SenderKey:         "alice@example.com",
		SenderLabel:       "Alice",
		Subject:           "Hello",
		Preview:           "Subject: Hello",
		EventJSON:         eventJSON,
	}
	if err := storage.DroppedEvents().Save(ctx, drop); err != nil {
		t.Fatal(err)
	}
	engine, err := commandset.NewBoundEngineForSource(commandengine.SourceMessage, nil, allowlistComponent)
	if err != nil {
		t.Fatalf("NewBoundEngineForSource() error = %v", err)
	}

	userReq := allowlistRequest(chat.ID, simplerbac.RoleUser)
	if _, err := engine.Run(ctx, userReq, []string{"allowlist", "whitelist", "alice@example.com"}); err == nil {
		t.Fatal("allowlist whitelist allowed non-root user")
	}

	rootReq := allowlistRequest(chat.ID, simplerbac.RoleRoot)
	view, err := engine.Run(ctx, rootReq, []string{"allowlist", "dropped", "view", drop.ID.String()})
	if err != nil {
		t.Fatalf("allowlist dropped view: %v", err)
	}
	if !strings.Contains(view.Text, "dropped event") || !strings.Contains(view.Text, "sender: alice@example.com") || !strings.Contains(view.Text, "subject: Hello") || !strings.Contains(view.Text, "Full body line two.") {
		t.Fatalf("unexpected view output:\n%s", view.Text)
	}
	add, err := engine.Run(ctx, rootReq, []string{"allowlist", "whitelist", "Alice <alice@example.com>"})
	if err != nil {
		t.Fatalf("allowlist whitelist: %v", err)
	}
	if !strings.Contains(add.Text, "alice@example.com") {
		t.Fatalf("unexpected add output: %q", add.Text)
	}
	list, err := engine.Run(ctx, rootReq, []string{"allowlist", "whitelist", "list"})
	if err != nil {
		t.Fatalf("allowlist whitelist list: %v", err)
	}
	if !strings.Contains(list.Text, "alice@example.com") {
		t.Fatalf("unexpected list output: %q", list.Text)
	}
	remove, err := engine.Run(ctx, rootReq, []string{"allowlist", "whitelist", "remove", "alice@example.com"})
	if err != nil {
		t.Fatalf("allowlist whitelist remove: %v", err)
	}
	if !strings.Contains(remove.Text, "removed") {
		t.Fatalf("unexpected remove output: %q", remove.Text)
	}
	list, err = engine.Run(ctx, rootReq, []string{"allowlist", "whitelist", "list"})
	if err != nil {
		t.Fatalf("allowlist whitelist list after remove: %v", err)
	}
	if !strings.Contains(list.Text, "empty") {
		t.Fatalf("unexpected empty list output: %q", list.Text)
	}
}

func seedAllowlistScope(t *testing.T, storage repository.Storage) (*coremodel.Chat, coremodel.ChatComponent) {
	t.Helper()
	ctx := context.Background()
	chat := &coremodel.Chat{Label: "team", Enabled: true}
	source := &coremodel.Component{Type: "telegram", Name: "telegram", Runtime: "local", Enabled: true}
	filter := &coremodel.Component{Type: Type, Name: Name, Runtime: "local", Enabled: true}
	for _, save := range []func(context.Context) error{
		func(ctx context.Context) error { return storage.Chats().Save(ctx, chat) },
		func(ctx context.Context) error { return storage.Components().Save(ctx, source) },
		func(ctx context.Context) error { return storage.Components().Save(ctx, filter) },
	} {
		if err := save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	sourceBinding := coremodel.ChatComponent{ChatID: chat.ID, ComponentID: source.ID, Role: coremodel.ChatComponentRoleSource, ExternalChannelID: "inbox", Enabled: true}
	if err := storage.ChatComponents().Save(ctx, &sourceBinding); err != nil {
		t.Fatal(err)
	}
	filterBinding := coremodel.InboundFilterBinding{SourceBindingID: sourceBinding.ID, FilterComponentID: filter.ID, Enabled: true}
	if err := storage.InboundFilterBindings().Save(ctx, &filterBinding); err != nil {
		t.Fatal(err)
	}
	return chat, sourceBinding
}

func allowlistRequest(chatID modeluuid.UUID, role simplerbac.Role) commandengine.Request {
	return commandengine.Request{Context: commandengine.Context{
		Source: commandengine.SourceMessage,
		Actor:  commandengine.Actor{ID: "tester", Roles: []simplerbac.Role{role}},
		ChatID: chatID,
	}}
}

func TestResolveDroppedEventIDReportsAmbiguity(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	allowlistComponent := New(storage)
	first, second, ref := ambiguousDropIDs()
	for _, id := range []modeluuid.UUID{first, second} {
		drop := &coremodel.DroppedEvent{ID: id, Status: "pending", Reason: "test"}
		if err := storage.DroppedEvents().Save(ctx, drop); err != nil {
			t.Fatal(err)
		}
	}
	_, err := allowlistComponent.resolveDroppedEventID(ctx, ref)
	if err == nil || !strings.Contains(err.Error(), "is ambiguous") || !strings.Contains(err.Error(), first.String()) || !strings.Contains(err.Error(), second.String()) {
		t.Fatalf("resolve error = %v, want ambiguous candidates", err)
	}
}

func ambiguousDropIDs() (modeluuid.UUID, modeluuid.UUID, string) {
	seen := map[string]modeluuid.UUID{}
	for {
		id := modeluuid.New()
		text := id.String()
		ref := text[len(text)-1:]
		if first, ok := seen[ref]; ok {
			return first, id, ref
		}
		seen[ref] = id
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
