package repository

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGORMStoragePersistsThreadMessagesAndArtifacts(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()

	chat := &coremodel.Chat{Label: "Codex #1"}
	if err := store.Chats().Save(ctx, chat); err != nil {
		t.Fatalf("save chat: %v", err)
	}

	thread := &coremodel.Thread{ChatID: chat.ID, Label: "general"}
	if err := store.Threads().Save(ctx, thread); err != nil {
		t.Fatalf("save thread: %v", err)
	}

	message := &coremodel.ThreadMessage{
		ChatID:     chat.ID,
		ThreadID:   thread.ID,
		Direction:  coremodel.DirectionInbound,
		Kind:       coremodel.MessageKindUser,
		SourceType: "telegram",
		ExternalID: "telegram:1:2:3",
		ActorID:    "13145044",
		ActorLabel: "@bartdeboer",
		Text:       "hello",
	}
	if err := store.Messages().Append(ctx, message); err != nil {
		t.Fatalf("append message: %v", err)
	}

	artifact := &coremodel.Artifact{
		ChatID:      chat.ID,
		ThreadID:    thread.ID,
		MessageID:   message.ID,
		Filename:    "hello.go",
		ContentType: "text/x-go",
		Syntax:      "go",
		Path:        "/artifacts/hello.go",
	}
	if err := store.Artifacts().Append(ctx, artifact); err != nil {
		t.Fatalf("append artifact: %v", err)
	}

	messages, err := store.Messages().ListByThreadID(ctx, thread.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 || messages[0].Text != "hello" || messages[0].Direction != coremodel.DirectionInbound {
		t.Fatalf("unexpected messages: %#v", messages)
	}

	artifacts, err := store.Artifacts().ListByMessageID(ctx, message.ID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Syntax != "go" {
		t.Fatalf("unexpected artifacts: %#v", artifacts)
	}
}

func TestGORMStorageMissingRecordsReturnNil(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()

	chat, err := store.Chats().GetByID(ctx, modeluuid.New())
	if err != nil {
		t.Fatalf("get missing chat: %v", err)
	}
	if chat != nil {
		t.Fatalf("expected nil chat, got %#v", chat)
	}
}

func TestGORMStorageEnsuresProviderChatAndThread(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()

	chat, err := store.Chats().EnsureProviderChat(ctx, " telegram ", " -1003759705932 ")
	if err != nil {
		t.Fatalf("ensure provider chat: %v", err)
	}
	again, err := store.Chats().EnsureProviderChat(ctx, "telegram", "-1003759705932")
	if err != nil {
		t.Fatalf("ensure provider chat again: %v", err)
	}
	if chat.ID != again.ID || chat.ProviderType != "telegram" || chat.ProviderChatID != "-1003759705932" {
		t.Fatalf("unexpected provider chat: first=%#v second=%#v", chat, again)
	}

	thread, err := store.Threads().EnsureProviderThread(ctx, chat.ID, " 845 ")
	if err != nil {
		t.Fatalf("ensure provider thread: %v", err)
	}
	threadAgain, err := store.Threads().EnsureProviderThread(ctx, chat.ID, "845")
	if err != nil {
		t.Fatalf("ensure provider thread again: %v", err)
	}
	if thread.ID != threadAgain.ID || thread.ProviderThreadID != "845" {
		t.Fatalf("unexpected provider thread: first=%#v second=%#v", thread, threadAgain)
	}
}

func TestGORMStorageListsDisabledChats(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()

	disabled, err := store.Chats().EnsureProviderChat(ctx, "telegram", "-100disabled")
	if err != nil {
		t.Fatalf("ensure disabled chat: %v", err)
	}
	enabled, err := store.Chats().EnsureProviderChat(ctx, "telegram", "-100enabled")
	if err != nil {
		t.Fatalf("ensure enabled chat: %v", err)
	}
	enabled.Enabled = true
	if err := store.Chats().Save(ctx, enabled); err != nil {
		t.Fatalf("save enabled chat: %v", err)
	}

	chats, err := store.Chats().ListDisabled(ctx)
	if err != nil {
		t.Fatalf("list disabled chats: %v", err)
	}
	if len(chats) != 1 || chats[0].ID != disabled.ID {
		t.Fatalf("disabled chats = %#v, want %#v", chats, disabled)
	}
}

func TestGORMStorageTransactionRollsBack(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	chat, err := store.Chats().EnsureProviderChat(ctx, "telegram", "-10042")
	if err != nil {
		t.Fatalf("ensure chat: %v", err)
	}

	wantErr := errors.New("rollback")
	err = store.Transaction(ctx, func(tx Storage) error {
		chat.Enabled = true
		if err := tx.Chats().Save(ctx, chat); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("transaction error = %v, want %v", err, wantErr)
	}
	got, err := store.Chats().GetByID(ctx, chat.ID)
	if err != nil {
		t.Fatalf("get chat: %v", err)
	}
	if got == nil || got.Enabled {
		t.Fatalf("transaction should roll back chat enable, got %#v", got)
	}
}

func TestGORMStoragePersistsComponentProfilesAndChatBindings(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()

	chat, err := store.Chats().EnsureProviderChat(ctx, "telegram", "-1003759705932")
	if err != nil {
		t.Fatalf("ensure chat: %v", err)
	}

	component := &coremodel.Component{Type: "codex", Label: "Codex", Enabled: true}
	if err := store.Components().Save(ctx, component); err != nil {
		t.Fatalf("save component: %v", err)
	}
	profile := &coremodel.ComponentProfile{ComponentType: "codex", ProfileName: "personal", Enabled: true}
	if err := store.ComponentProfiles().Save(ctx, profile); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	binding := &coremodel.ChatComponent{ChatID: chat.ID, ComponentType: "codex", ProfileName: "personal", Enabled: true}
	if err := store.ChatComponents().Save(ctx, binding); err != nil {
		t.Fatalf("save chat component: %v", err)
	}

	gotComponent, err := store.Components().GetByType(ctx, "codex")
	if err != nil {
		t.Fatalf("get component: %v", err)
	}
	if gotComponent == nil || gotComponent.ID != component.ID {
		t.Fatalf("unexpected component: %#v", gotComponent)
	}

	gotProfile, err := store.ComponentProfiles().Get(ctx, "codex", "personal")
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if gotProfile == nil || gotProfile.ID != profile.ID {
		t.Fatalf("unexpected profile: %#v", gotProfile)
	}

	bindings, err := store.ChatComponents().ListEnabledByChatID(ctx, chat.ID)
	if err != nil {
		t.Fatalf("list enabled bindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].ComponentType != "codex" || bindings[0].ProfileName != "personal" {
		t.Fatalf("unexpected bindings: %#v", bindings)
	}
}

func TestGORMStorageComponentSavesAreIdempotent(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()

	chat, err := store.Chats().EnsureProviderChat(ctx, "telegram", "-1003759705932")
	if err != nil {
		t.Fatalf("ensure chat: %v", err)
	}

	component := &coremodel.Component{Type: " codex ", Enabled: true}
	if err := store.Components().Save(ctx, component); err != nil {
		t.Fatalf("save component: %v", err)
	}
	componentAgain := &coremodel.Component{Type: "codex", Enabled: true}
	if err := store.Components().Save(ctx, componentAgain); err != nil {
		t.Fatalf("save component again: %v", err)
	}
	if componentAgain.ID != component.ID {
		t.Fatalf("component IDs differ: first=%s second=%s", component.ID, componentAgain.ID)
	}

	profile := &coremodel.ComponentProfile{ComponentType: " codex ", ProfileName: " v2test ", Enabled: true}
	if err := store.ComponentProfiles().Save(ctx, profile); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	profileAgain := &coremodel.ComponentProfile{ComponentType: "codex", ProfileName: "v2test", Enabled: true}
	if err := store.ComponentProfiles().Save(ctx, profileAgain); err != nil {
		t.Fatalf("save profile again: %v", err)
	}
	if profileAgain.ID != profile.ID {
		t.Fatalf("profile IDs differ: first=%s second=%s", profile.ID, profileAgain.ID)
	}

	binding := &coremodel.ChatComponent{ChatID: chat.ID, ComponentType: " codex ", ProfileName: " v2test ", Enabled: true}
	if err := store.ChatComponents().Save(ctx, binding); err != nil {
		t.Fatalf("save binding: %v", err)
	}
	bindingAgain := &coremodel.ChatComponent{ChatID: chat.ID, ComponentType: "codex", ProfileName: "v2test", Enabled: true}
	if err := store.ChatComponents().Save(ctx, bindingAgain); err != nil {
		t.Fatalf("save binding again: %v", err)
	}
	if bindingAgain.ID != binding.ID {
		t.Fatalf("binding IDs differ: first=%s second=%s", binding.ID, bindingAgain.ID)
	}

	thread, err := store.Threads().EnsureProviderThread(ctx, chat.ID, "845")
	if err != nil {
		t.Fatalf("ensure thread: %v", err)
	}
	state := &coremodel.ThreadComponentState{ThreadID: thread.ID, ComponentType: " codex ", ProfileName: " v2test ", StateJSON: `{"thread_id":"codex-1"}`}
	if err := store.ThreadComponentStates().Save(ctx, state); err != nil {
		t.Fatalf("save thread component state: %v", err)
	}
	stateAgain := &coremodel.ThreadComponentState{ThreadID: thread.ID, ComponentType: "codex", ProfileName: "v2test", StateJSON: `{"thread_id":"codex-2"}`}
	if err := store.ThreadComponentStates().Save(ctx, stateAgain); err != nil {
		t.Fatalf("save thread component state again: %v", err)
	}
	if stateAgain.ID != state.ID {
		t.Fatalf("thread state IDs differ: first=%s second=%s", state.ID, stateAgain.ID)
	}
	gotState, err := store.ThreadComponentStates().Get(ctx, thread.ID, "codex", "v2test")
	if err != nil {
		t.Fatalf("get thread component state: %v", err)
	}
	if gotState == nil || gotState.StateJSON != `{"thread_id":"codex-2"}` {
		t.Fatalf("unexpected thread component state: %#v", gotState)
	}
}

func newTestStore(t *testing.T) *GORMStorage {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ctgbot-v2.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	store := NewGORM(db)
	if err := store.AutoMigrate(context.Background()); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return store
}
