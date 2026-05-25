package gormstorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTransactionRollsBackOnError(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	chat := &coremodel.Chat{Label: "before", Enabled: true}
	if err := store.Chats().Save(ctx, chat); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	wantErr := errors.New("boom")
	err := store.Transaction(ctx, func(tx repository.Storage) error {
		copy := *chat
		copy.Label = "after"
		if err := tx.Chats().Save(ctx, &copy); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Transaction() error = %v, want %v", err, wantErr)
	}

	stored, err := store.Chats().GetByID(ctx, chat.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if stored == nil {
		t.Fatal("expected stored chat")
	}
	if got, want := stored.Label, "before"; got != want {
		t.Fatalf("Label after rollback = %q, want %q", got, want)
	}
}

func TestTransactionCommitsOnSuccess(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	chat := &coremodel.Chat{Label: "before", Enabled: true}
	if err := store.Chats().Save(ctx, chat); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	err := store.Transaction(ctx, func(tx repository.Storage) error {
		copy := *chat
		copy.Label = "after"
		return tx.Chats().Save(ctx, &copy)
	})
	if err != nil {
		t.Fatalf("Transaction() error = %v", err)
	}

	stored, err := store.Chats().GetByID(ctx, chat.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if stored == nil {
		t.Fatal("expected stored chat")
	}
	if got, want := stored.Label, "after"; got != want {
		t.Fatalf("Label after commit = %q, want %q", got, want)
	}
}

func TestChatsShortIDs(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	first := fixedUUID(1)
	second := fixedUUID(2)

	for _, chat := range []*coremodel.Chat{
		{ID: first, Label: "first", Enabled: true},
		{ID: second, Label: "second", Enabled: true},
	} {
		if err := store.Chats().Save(ctx, chat); err != nil {
			t.Fatalf("Chats().Save() error = %v", err)
		}
	}

	ids, err := store.Chats().ListIDs(ctx)
	if err != nil {
		t.Fatalf("Chats().ListIDs() error = %v", err)
	}
	resolver := repository.NewShortIDResolver(ids)

	shortID, err := resolver.ShortIDFor(first, 1)
	if err != nil {
		t.Fatalf("ShortIDFor() error = %v", err)
	}
	if !strings.HasSuffix(first.String(), shortID) {
		t.Fatalf("short ID %q is not a suffix of %s", shortID, first)
	}
	resolved, err := resolver.Resolve(shortID)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved != first {
		t.Fatalf("resolved = %s, want %s", resolved, first)
	}
}

func TestThreadsShortIDs(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	chatID := modeluuid.New()
	first := fixedUUID(1)
	second := fixedUUID(2)

	for _, thread := range []*coremodel.Thread{
		{ID: first, ChatID: chatID, Label: "first"},
		{ID: second, ChatID: chatID, Label: "second"},
	} {
		if err := store.Threads().Save(ctx, thread); err != nil {
			t.Fatalf("Threads().Save() error = %v", err)
		}
	}

	ids, err := store.Threads().ListIDs(ctx)
	if err != nil {
		t.Fatalf("Threads().ListIDs() error = %v", err)
	}
	resolver := repository.NewShortIDResolver(ids)

	shortID, err := resolver.ShortIDFor(first, 1)
	if err != nil {
		t.Fatalf("ShortIDFor() error = %v", err)
	}
	if !strings.HasSuffix(first.String(), shortID) {
		t.Fatalf("short ID %q is not a suffix of %s", shortID, first)
	}
	if shortID == first.String() {
		t.Fatalf("short ID = full ID %q, want shortest unique suffix", shortID)
	}

	resolved, err := resolver.Resolve(shortID)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved != first {
		t.Fatalf("resolved = %s, want %s", resolved, first)
	}

	_, err = resolver.Resolve("0")
	var ambiguous *repository.ShortIDAmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("Resolve(\"0\") error = %v, want ambiguous", err)
	}
}

func TestArtifactsStoreContentOnDiskOnly(t *testing.T) {
	ctx := context.Background()
	store := newTestStoreWithArtifactDir(t)

	messageID := modeluuid.New()
	artifact := &coremodel.Artifact{
		ChatID:      modeluuid.New(),
		ThreadID:    modeluuid.New(),
		MessageID:   messageID,
		ComponentID: modeluuid.New(),
		Filename:    "stdin.txt",
		ContentType: "text/plain",
		Content:     []byte("hello from disk"),
	}
	if err := store.Artifacts().Append(ctx, artifact); err != nil {
		t.Fatalf("Artifacts().Append() error = %v", err)
	}
	if got, want := artifact.StorageKind, "file"; got != want {
		t.Fatalf("StorageKind = %q, want %q", got, want)
	}
	if artifact.StoragePath == "" {
		t.Fatal("StoragePath is empty")
	}
	if got, want := artifact.Size, int64(len("hello from disk")); got != want {
		t.Fatalf("Size = %d, want %d", got, want)
	}
	if artifact.SHA256 == "" {
		t.Fatal("SHA256 is empty")
	}
	if len(artifact.Content) != 0 {
		t.Fatalf("artifact kept %d bytes in memory after save, want 0", len(artifact.Content))
	}
	onDisk, err := os.ReadFile(filepath.Join(store.artifacts.artifactDir, artifact.StoragePath))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(onDisk), "hello from disk"; got != want {
		t.Fatalf("disk content = %q, want %q", got, want)
	}

	var raw struct {
		StorageKind string `gorm:"column:storage_kind"`
		StoragePath string `gorm:"column:storage_path"`
		Size        int64  `gorm:"column:size"`
		SHA256      string `gorm:"column:sha256"`
	}
	if err := store.db.WithContext(ctx).
		Table("artifacts").
		Select("storage_kind, storage_path, size, sha256").
		Where("id = ?", artifact.ID).
		Scan(&raw).Error; err != nil {
		t.Fatalf("raw artifact metadata load error = %v", err)
	}
	if raw.StorageKind != "file" || raw.StoragePath == "" || raw.Size == 0 || raw.SHA256 == "" {
		t.Fatalf("raw artifact metadata = %#v", raw)
	}

	loaded, err := store.Artifacts().ListByMessageID(ctx, messageID)
	if err != nil {
		t.Fatalf("Artifacts().ListByMessageID() error = %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loaded artifacts = %d, want 1", len(loaded))
	}
	if got, want := string(loaded[0].Content), "hello from disk"; got != want {
		t.Fatalf("loaded content = %q, want %q", got, want)
	}
}

func TestArtifactsAppendFailsWithoutArtifactDir(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	err := store.Artifacts().Append(ctx, &coremodel.Artifact{
		ChatID:      modeluuid.New(),
		ThreadID:    modeluuid.New(),
		MessageID:   modeluuid.New(),
		ComponentID: modeluuid.New(),
		Filename:    "stdin.txt",
		ContentType: "text/plain",
		Content:     []byte("hello"),
	})
	if err == nil {
		t.Fatal("Artifacts().Append() unexpectedly succeeded without artifact dir")
	}
	if !strings.Contains(err.Error(), "artifact storage directory is not configured") {
		t.Fatalf("Artifacts().Append() error = %v", err)
	}
}

func TestTransactionPreservesArtifactDir(t *testing.T) {
	ctx := context.Background()
	store := newTestStoreWithArtifactDir(t)
	messageID := modeluuid.New()

	err := store.Transaction(ctx, func(tx repository.Storage) error {
		return tx.Artifacts().Append(ctx, &coremodel.Artifact{
			ChatID:      modeluuid.New(),
			ThreadID:    modeluuid.New(),
			MessageID:   messageID,
			ComponentID: modeluuid.New(),
			Filename:    "stdin.txt",
			ContentType: "text/plain",
			Content:     []byte("hello from transaction"),
		})
	})
	if err != nil {
		t.Fatalf("Transaction(artifact append) error = %v", err)
	}

	artifacts, err := store.Artifacts().ListByMessageID(ctx, messageID)
	if err != nil {
		t.Fatalf("Artifacts().ListByMessageID() error = %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("len(artifacts) = %d, want 1", len(artifacts))
	}
	if got, want := string(artifacts[0].Content), "hello from transaction"; got != want {
		t.Fatalf("artifact content = %q, want %q", got, want)
	}
}

func TestInboundDropsSaveListGetDelete(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	componentID := modeluuid.New()
	now := time.Now()

	drop := &coremodel.InboundDrop{
		ComponentID:       componentID,
		ExternalChannelID: "chat-1",
		ExternalThreadID:  "thread-9",
		ChatLabel:         "New chat",
		ActorID:           "bart",
		ActorLabel:        "Bart",
		LastTextPreview:   "hello",
		MessageCount:      2,
		FirstSeenAt:       now.Add(-time.Minute),
		LastSeenAt:        now,
	}
	if err := store.InboundDrops().Save(ctx, drop); err != nil {
		t.Fatalf("InboundDrops().Save() error = %v", err)
	}
	if drop.ID.IsNull() {
		t.Fatal("InboundDrops().Save() did not assign ID")
	}

	loaded, err := store.InboundDrops().GetByComponentAndExternalChannelID(ctx, componentID, "chat-1")
	if err != nil {
		t.Fatalf("InboundDrops().GetByComponentAndExternalChannelID() error = %v", err)
	}
	if loaded == nil {
		t.Fatal("InboundDrops().GetByComponentAndExternalChannelID() = nil, want row")
	}
	if got, want := loaded.MessageCount, int64(2); got != want {
		t.Fatalf("MessageCount = %d, want %d", got, want)
	}

	list, err := store.InboundDrops().List(ctx)
	if err != nil {
		t.Fatalf("InboundDrops().List() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("InboundDrops().List() len = %d, want 1", len(list))
	}

	if err := store.InboundDrops().DeleteByComponentAndExternalChannelID(ctx, componentID, "chat-1"); err != nil {
		t.Fatalf("InboundDrops().DeleteByComponentAndExternalChannelID() error = %v", err)
	}
	loaded, err = store.InboundDrops().GetByComponentAndExternalChannelID(ctx, componentID, "chat-1")
	if err != nil {
		t.Fatalf("InboundDrops().GetByComponentAndExternalChannelID() after delete error = %v", err)
	}
	if loaded != nil {
		t.Fatalf("InboundDrops().GetByComponentAndExternalChannelID() after delete = %#v, want nil", loaded)
	}
}

func TestDroppedEventsAndAllowlistSenders(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	sourceBindingID := modeluuid.New()
	filterComponentID := modeluuid.New()

	filterBinding := &coremodel.InboundFilterBinding{SourceBindingID: sourceBindingID, FilterComponentID: filterComponentID, Enabled: true}
	if err := store.InboundFilterBindings().Save(ctx, filterBinding); err != nil {
		t.Fatalf("InboundFilterBindings().Save() error = %v", err)
	}
	loadedFilterBinding, err := store.InboundFilterBindings().GetBySourceBindingAndFilter(ctx, sourceBindingID, filterComponentID)
	if err != nil {
		t.Fatalf("InboundFilterBindings().GetBySourceBindingAndFilter() error = %v", err)
	}
	if loadedFilterBinding == nil || loadedFilterBinding.ID.IsNull() {
		t.Fatalf("loaded inbound filter binding = %#v", loadedFilterBinding)
	}
	filterBindings, err := store.InboundFilterBindings().ListEnabledBySourceBindingID(ctx, sourceBindingID)
	if err != nil {
		t.Fatalf("InboundFilterBindings().ListEnabledBySourceBindingID() error = %v", err)
	}
	if len(filterBindings) != 1 {
		t.Fatalf("inbound filter binding count = %d, want 1", len(filterBindings))
	}

	drop := &coremodel.DroppedEvent{
		Status:            "pending",
		Action:            "drop",
		Reason:            "allowlist-unknown-sender",
		SourceBindingID:   sourceBindingID,
		ComponentID:       modeluuid.New(),
		ProviderChannelID: "inbox",
		SenderKey:         "alice@example.com",
		ExpiresAt:         time.Now().Add(time.Hour),
	}
	if err := store.DroppedEvents().Save(ctx, drop); err != nil {
		t.Fatalf("DroppedEvents().Save() error = %v", err)
	}
	if drop.ID.IsNull() {
		t.Fatal("DroppedEvents().Save() did not assign ID")
	}
	loadedDrop, err := store.DroppedEvents().GetByID(ctx, drop.ID)
	if err != nil {
		t.Fatalf("DroppedEvents().GetByID() error = %v", err)
	}
	if loadedDrop == nil || loadedDrop.SenderKey != "alice@example.com" {
		t.Fatalf("loaded dropped event = %#v", loadedDrop)
	}
	deleted, err := store.DroppedEvents().DeleteExpired(ctx, time.Now().Add(2*time.Hour))
	if err != nil {
		t.Fatalf("DroppedEvents().DeleteExpired() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted dropped events = %d, want 1", deleted)
	}

	sender := &coremodel.AllowlistSender{SourceBindingID: sourceBindingID, SenderKey: "alice@example.com", SenderLabel: "Alice"}
	if err := store.AllowlistSenders().Save(ctx, sender); err != nil {
		t.Fatalf("AllowlistSenders().Save() error = %v", err)
	}
	loadedSender, err := store.AllowlistSenders().GetBySourceBindingAndSenderKey(ctx, sourceBindingID, "alice@example.com")
	if err != nil {
		t.Fatalf("AllowlistSenders().GetBySourceBindingAndSenderKey() error = %v", err)
	}
	if loadedSender == nil || loadedSender.SenderLabel != "Alice" {
		t.Fatalf("loaded sender = %#v", loadedSender)
	}
	list, err := store.AllowlistSenders().ListBySourceBindingID(ctx, sourceBindingID)
	if err != nil {
		t.Fatalf("AllowlistSenders().ListBySourceBindingID() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("allowlist sender count = %d, want 1", len(list))
	}
	removed, err := store.AllowlistSenders().DeleteBySourceBindingAndSenderKey(ctx, sourceBindingID, "alice@example.com")
	if err != nil {
		t.Fatalf("AllowlistSenders().DeleteBySourceBindingAndSenderKey() error = %v", err)
	}
	if !removed {
		t.Fatal("AllowlistSenders().DeleteBySourceBindingAndSenderKey() removed=false, want true")
	}
}

func TestAutoMigrateRenamesExternalChannelColumns(t *testing.T) {
	ctx := context.Background()
	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%s?mode=memory&cache=shared", name, modeluuid.New().String())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}

	for _, stmt := range []string{
		`CREATE TABLE chat_components (
			id BLOB PRIMARY KEY,
			chat_id BLOB,
			component_id BLOB,
			role TEXT,
			external_chat_id TEXT,
			enabled NUMERIC,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE inbound_drops (
			id BLOB PRIMARY KEY,
			component_id BLOB,
			external_chat_id TEXT,
			external_thread_id TEXT,
			chat_label TEXT,
			actor_id TEXT,
			actor_label TEXT,
			last_text_preview TEXT,
			message_count INTEGER,
			first_seen_at DATETIME,
			last_seen_at DATETIME
		)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create legacy table: %v", err)
		}
	}

	chatID := modeluuid.New()
	componentID := modeluuid.New()
	if err := db.Exec(
		`INSERT INTO chat_components (id, chat_id, component_id, role, external_chat_id, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		modeluuid.New(), chatID, componentID, string(coremodel.ChatComponentRoleSource), "legacy-channel", true, time.Now(), time.Now(),
	).Error; err != nil {
		t.Fatalf("insert legacy chat component: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO inbound_drops (id, component_id, external_chat_id, external_thread_id, chat_label, actor_id, actor_label, last_text_preview, message_count, first_seen_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		modeluuid.New(), componentID, "legacy-channel", "thread-1", "Legacy", "actor-1", "Actor", "hello", 3, time.Now(), time.Now(),
	).Error; err != nil {
		t.Fatalf("insert legacy inbound drop: %v", err)
	}

	store := New(db)
	if err := store.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if db.Migrator().HasColumn(&coremodel.ChatComponent{}, "external_chat_id") {
		t.Fatal("chat_components.external_chat_id still exists")
	}
	if !db.Migrator().HasColumn(&coremodel.ChatComponent{}, "external_channel_id") {
		t.Fatal("chat_components.external_channel_id missing")
	}
	if db.Migrator().HasColumn(&coremodel.InboundDrop{}, "external_chat_id") {
		t.Fatal("inbound_drops.external_chat_id still exists")
	}
	if !db.Migrator().HasColumn(&coremodel.InboundDrop{}, "external_channel_id") {
		t.Fatal("inbound_drops.external_channel_id missing")
	}

	binding, err := store.ChatComponents().FindByComponentRoleAndExternalChannelID(ctx, componentID, coremodel.ChatComponentRoleSource, "legacy-channel")
	if err != nil {
		t.Fatalf("FindByComponentRoleAndExternalChannelID() error = %v", err)
	}
	if binding == nil || binding.ChatID != chatID {
		t.Fatalf("binding after migration = %#v, want chat %s", binding, chatID)
	}
	drop, err := store.InboundDrops().GetByComponentAndExternalChannelID(ctx, componentID, "legacy-channel")
	if err != nil {
		t.Fatalf("GetByComponentAndExternalChannelID() error = %v", err)
	}
	if drop == nil || drop.MessageCount != 3 {
		t.Fatalf("drop after migration = %#v, want message_count 3", drop)
	}
}

func TestAutoMigrateBackfillsThreadMessageRoles(t *testing.T) {
	ctx := context.Background()
	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%s?mode=memory&cache=shared", name, modeluuid.New().String())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	if err := db.Exec(`CREATE TABLE thread_messages (
		id BLOB PRIMARY KEY,
		chat_id BLOB,
		thread_id BLOB,
		direction TEXT,
		kind TEXT,
		component_id BLOB,
		external_id TEXT,
		actor_id TEXT,
		actor_label TEXT,
		text TEXT,
		metadata_json TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create legacy thread_messages: %v", err)
	}
	messages := []struct {
		id        modeluuid.UUID
		direction coremodel.MessageDirection
		kind      coremodel.MessageKind
		want      coremodel.MessageRole
	}{
		{id: modeluuid.New(), direction: coremodel.MessageDirectionInbound, kind: coremodel.MessageKindUser, want: coremodel.MessageRoleUser},
		{id: modeluuid.New(), direction: coremodel.MessageDirectionOutbound, kind: coremodel.MessageKindAgent, want: coremodel.MessageRoleAgent},
		{id: modeluuid.New(), direction: coremodel.MessageDirectionOutbound, kind: coremodel.MessageKindSystem, want: coremodel.MessageRoleSystem},
		{id: modeluuid.New(), direction: coremodel.MessageDirectionInbound, kind: coremodel.MessageKindEvent, want: coremodel.MessageRoleSystem},
		{id: modeluuid.New(), direction: coremodel.MessageDirectionInbound, kind: coremodel.MessageKindMessage, want: coremodel.MessageRoleUser},
	}
	for _, message := range messages {
		if err := db.Exec(
			`INSERT INTO thread_messages (id, chat_id, thread_id, direction, kind, text, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			message.id, modeluuid.New(), modeluuid.New(), string(message.direction), string(message.kind), "hello", time.Now(), time.Now(),
		).Error; err != nil {
			t.Fatalf("insert message: %v", err)
		}
	}

	store := New(db)
	if err := store.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if !db.Migrator().HasColumn(&coremodel.ThreadMessage{}, "role") {
		t.Fatal("thread_messages.role missing")
	}
	for _, message := range messages {
		var got string
		if err := db.Raw(`SELECT role FROM thread_messages WHERE id = ?`, message.id).Scan(&got).Error; err != nil {
			t.Fatalf("select role: %v", err)
		}
		if got != string(message.want) {
			t.Fatalf("role for kind=%s direction=%s = %q, want %q", message.kind, message.direction, got, message.want)
		}
	}
}

func newTestStore(t *testing.T) *GORMStorage {
	t.Helper()
	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%s?mode=memory&cache=shared", name, modeluuid.New().String())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	store := New(db)
	if err := store.AutoMigrate(context.Background()); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	return store
}

func fixedUUID(last byte) modeluuid.UUID {
	var id modeluuid.UUID
	id[6] = last
	return id
}

func newTestStoreWithArtifactDir(t *testing.T) *GORMStorage {
	t.Helper()
	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%s?mode=memory&cache=shared", name, modeluuid.New().String())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	store := NewWithArtifactDir(db, filepath.Join(t.TempDir(), "artifacts"))
	if err := store.AutoMigrate(context.Background()); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	return store
}
