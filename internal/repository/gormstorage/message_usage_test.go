package gormstorage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestFinalUsageSQLiteReopenIsolationAndDeletedRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")
	open := func() *GORMStorage {
		db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		s := New(db)
		if err := s.AutoMigrate(context.Background()); err != nil {
			t.Fatal(err)
		}
		return s
	}
	store := open()
	ctx := t.Context()
	thread, component, other := modeluuid.New(), modeluuid.New(), modeluuid.New()
	zero := int64(0)
	first := coremodel.ThreadMessage{ThreadID: thread, ComponentID: component, Text: "same", CreatedAt: time.Now().Add(-72 * time.Hour)}
	if err := store.Messages().Append(ctx, &first); err != nil {
		t.Fatal(err)
	}
	usage := coremodel.MessageUsage{ProviderSessionID: "session-a", Scope: "turn", InputTokens: &zero}
	if err := store.Messages().Finalize(ctx, first.ID, thread, other, usage); err == nil {
		t.Fatal("wrong component accepted")
	}
	if err := store.Messages().Finalize(ctx, first.ID, thread, component, usage); err != nil {
		t.Fatal(err)
	}
	if err := store.Messages().Finalize(ctx, first.ID, thread, component, usage); err == nil {
		t.Fatal("duplicate finalization accepted")
	}
	for _, m := range []coremodel.ThreadMessage{{ThreadID: thread, ComponentID: other, IsFinal: true}, {ThreadID: modeluuid.New(), ComponentID: component, IsFinal: true}, {ThreadID: thread, ComponentID: component, Text: "intermediate"}} {
		m := m
		if err := store.Messages().Append(ctx, &m); err != nil {
			t.Fatal(err)
		}
	}
	state := coremodel.ThreadComponentState{ThreadID: thread, ComponentID: component, StateJSON: `{"model":"x"}`}
	if err := store.ThreadComponentStates().Save(ctx, &state); err != nil {
		t.Fatal(err)
	}
	if err := store.ThreadComponentStates().DeleteByThreadAndComponent(ctx, thread, component); err != nil {
		t.Fatal(err)
	}
	db, _ := store.db.DB()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store = open()
	defer func() { db, _ := store.db.DB(); _ = db.Close() }()
	got, err := store.Messages().LatestFinal(ctx, thread, component)
	if err != nil || got == nil || got.ID != first.ID || got.Usage.InputTokens == nil || *got.Usage.InputTokens != 0 || got.Usage.OutputTokens != nil {
		t.Fatal(got, err)
	}
	// A newer finalized reply with unavailable usage must not display older counts.
	newer := coremodel.ThreadMessage{ThreadID: thread, ComponentID: component, IsFinal: true, Usage: coremodel.MessageUsage{ProviderSessionID: "session-a"}}
	if err := store.Messages().Append(ctx, &newer); err != nil {
		t.Fatal(err)
	}
	got, err = store.Messages().LatestFinal(ctx, thread, component)
	if err != nil || got.ID != newer.ID || got.Usage.InputTokens != nil {
		t.Fatal(got, err)
	}
	if _, err := store.Messages().DeleteByThreadID(ctx, thread); err != nil {
		t.Fatal(err)
	}
	if err := store.Messages().Finalize(ctx, newer.ID, thread, component, usage); err == nil {
		t.Fatal("late result recreated deleted message")
	}
	got, err = store.Messages().LatestFinal(ctx, thread, component)
	if err != nil || got != nil {
		t.Fatal(got, err)
	}
}
