package repository

import (
	"context"
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
