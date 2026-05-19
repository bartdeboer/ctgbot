package messagingcomponent

import (
	"context"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/buildassets"
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

	result, err := engine.Run(ctx, testMessagingRequest(thread.ID, simplerbac.RoleRoot), []string{"status"})
	if err != nil {
		t.Fatalf("Run(status) error = %v", err)
	}
	chatShortID := shortChatIDForMessagingTest(t, storage, thread.ChatID)
	for _, want := range []string{
		"thread status",
		"ctgbot_version: " + buildassets.Version(),
		"chat_short_id: " + chatShortID,
		"chat_label: Codex #1",
		"thread_label: ctgbot 2",
		"- telegram source external_channel_id=-100 external_thread_id=845",
	} {
		if !strings.Contains(result.Text, want) {
			t.Fatalf("status missing %q:\n%s", want, result.Text)
		}
	}
}

func shortChatIDForMessagingTest(t *testing.T, storage repository.Storage, chatID modeluuid.UUID) string {
	t.Helper()
	ids, err := storage.Chats().ListIDs(context.Background())
	if err != nil {
		t.Fatalf("Chats().ListIDs() error = %v", err)
	}
	shortID, err := repository.NewShortIDResolver(ids).ShortIDFor(chatID, 6)
	if err != nil {
		t.Fatalf("ShortIDFor(%s) error = %v", chatID, err)
	}
	return shortID
}

func TestThreadCurrentStatusAllowsUser(t *testing.T) {
	ctx := context.Background()
	storage, thread := testMessagingStorage(t, ctx)
	engine := testMessagingEngine(t, storage)

	for _, argv := range [][]string{
		{"status"},
		{"thread", "status"},
		{"thread", "current", "status"},
	} {
		if _, err := engine.Run(ctx, testMessagingRequest(thread.ID, simplerbac.RoleUser), argv); err != nil {
			t.Fatalf("Run(%v) error = %v, want user to read current status", argv, err)
		}
	}
}

func TestThreadReferencedStatusDeniesUser(t *testing.T) {
	ctx := context.Background()
	storage, thread := testMessagingStorage(t, ctx)
	engine := testMessagingEngine(t, storage)

	_, err := engine.Run(ctx, testMessagingRequest(thread.ID, simplerbac.RoleUser), []string{"thread", thread.ID.String(), "status"})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("Run(thread <thread> status) error = %v, want denied", err)
	}
}

func TestThreadLabelSetUpdatesCurrentThread(t *testing.T) {
	ctx := context.Background()
	storage, thread := testMessagingStorage(t, ctx)
	engine := testMessagingEngine(t, storage)

	result, err := engine.Run(ctx, testMessagingRequest(thread.ID, simplerbac.RoleRoot), []string{"thread", "label", "set", "new", "label"})
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

func TestThreadLabelSetDeniesUser(t *testing.T) {
	ctx := context.Background()
	storage, thread := testMessagingStorage(t, ctx)
	engine := testMessagingEngine(t, storage)

	_, err := engine.Run(ctx, testMessagingRequest(thread.ID, simplerbac.RoleUser), []string{"thread", "label", "set", "new", "label"})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("Run(thread label set) error = %v, want denied", err)
	}
}

func TestThreadComponentBindInfersProviderThreadIDFromSourceBinding(t *testing.T) {
	ctx := context.Background()
	storage, thread := testMessagingStorage(t, ctx)
	gmail := testRegisterComponent(t, ctx, storage, "gmail", "personal")
	testSaveChatComponent(t, ctx, storage, coremodel.ChatComponent{
		ChatID:            thread.ChatID,
		ComponentID:       gmail.ID,
		Role:              coremodel.ChatComponentRoleSource,
		ExternalChannelID: "bart@example.com",
		Enabled:           true,
	})
	engine := testMessagingEngine(t, storage)

	result, err := engine.Run(ctx, testMessagingRequest(thread.ID, simplerbac.RoleRoot), []string{"thread", "component", "bind", "gmail/personal"})
	if err != nil {
		t.Fatalf("Run(thread component bind) error = %v", err)
	}
	for _, want := range []string{
		"thread component bound",
		"thread_id: " + thread.ID.String(),
		"component: gmail/personal",
		"provider_thread_id: bart@example.com",
	} {
		if !strings.Contains(result.Text, want) {
			t.Fatalf("result missing %q:\n%s", want, result.Text)
		}
	}
	assertThreadComponentMapping(t, ctx, storage, thread.ID, gmail.ID, "bart@example.com")
}

func TestThreadComponentBindExplicitProviderThreadID(t *testing.T) {
	ctx := context.Background()
	storage, thread := testMessagingStorage(t, ctx)
	gmail := testRegisterComponent(t, ctx, storage, "gmail", "personal")
	engine := testMessagingEngine(t, storage)

	result, err := engine.Run(ctx, testMessagingRequest(modeluuid.Nil, simplerbac.RoleRoot), []string{"thread", thread.ID.String(), "component", "bind", "gmail/personal", "gmail-thread-123"})
	if err != nil {
		t.Fatalf("Run(thread component bind explicit) error = %v", err)
	}
	if !strings.Contains(result.Text, "provider_thread_id: gmail-thread-123") {
		t.Fatalf("result = %q, want explicit provider thread id", result.Text)
	}
	assertThreadComponentMapping(t, ctx, storage, thread.ID, gmail.ID, "gmail-thread-123")
}

func TestThreadComponentBindErrorsWhenProviderThreadIDMissing(t *testing.T) {
	ctx := context.Background()
	storage, thread := testMessagingStorage(t, ctx)
	testRegisterComponent(t, ctx, storage, "gmail", "personal")
	engine := testMessagingEngine(t, storage)

	_, err := engine.Run(ctx, testMessagingRequest(thread.ID, simplerbac.RoleRoot), []string{"thread", "component", "bind", "gmail/personal"})
	if err == nil || !strings.Contains(err.Error(), "cannot infer provider thread id") {
		t.Fatalf("Run(thread component bind missing) error = %v, want inference error", err)
	}
}

func TestThreadComponentBindErrorsWhenProviderThreadIDAmbiguous(t *testing.T) {
	ctx := context.Background()
	storage, thread := testMessagingStorage(t, ctx)
	gmail := testRegisterComponent(t, ctx, storage, "gmail", "personal")
	for _, externalChannelID := range []string{"mailbox-a", "mailbox-b"} {
		testSaveChatComponent(t, ctx, storage, coremodel.ChatComponent{
			ChatID:            thread.ChatID,
			ComponentID:       gmail.ID,
			Role:              coremodel.ChatComponentRoleSource,
			ExternalChannelID: externalChannelID,
			Enabled:           true,
		})
	}
	engine := testMessagingEngine(t, storage)

	_, err := engine.Run(ctx, testMessagingRequest(thread.ID, simplerbac.RoleRoot), []string{"thread", "component", "bind", "gmail/personal"})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("Run(thread component bind ambiguous) error = %v, want ambiguous error", err)
	}
}

func TestThreadComponentBindDedupesDuplicateInferredProviderThreadIDs(t *testing.T) {
	ctx := context.Background()
	storage, thread := testMessagingStorage(t, ctx)
	gmail := testRegisterComponent(t, ctx, storage, "gmail", "personal")
	for i := 0; i < 2; i++ {
		testSaveChatComponent(t, ctx, storage, coremodel.ChatComponent{
			ChatID:            thread.ChatID,
			ComponentID:       gmail.ID,
			Role:              coremodel.ChatComponentRoleSource,
			ExternalChannelID: "bart@example.com",
			Enabled:           true,
		})
	}
	engine := testMessagingEngine(t, storage)

	if _, err := engine.Run(ctx, testMessagingRequest(thread.ID, simplerbac.RoleRoot), []string{"thread", "component", "bind", "gmail/personal"}); err != nil {
		t.Fatalf("Run(thread component bind duplicate inferred ids) error = %v", err)
	}
	assertThreadComponentMapping(t, ctx, storage, thread.ID, gmail.ID, "bart@example.com")
}

func TestThreadComponentBindIsIdempotent(t *testing.T) {
	ctx := context.Background()
	storage, thread := testMessagingStorage(t, ctx)
	gmail := testRegisterComponent(t, ctx, storage, "gmail", "personal")
	if err := storage.ThreadComponentMappings().Save(ctx, &coremodel.ThreadComponentMapping{
		ThreadID:          thread.ID,
		ChatID:            thread.ChatID,
		ComponentID:       gmail.ID,
		ComponentThreadID: "bart@example.com",
	}); err != nil {
		t.Fatalf("Save(mapping) error = %v", err)
	}
	engine := testMessagingEngine(t, storage)

	if _, err := engine.Run(ctx, testMessagingRequest(thread.ID, simplerbac.RoleRoot), []string{"thread", "component", "bind", "gmail/personal", "bart@example.com"}); err != nil {
		t.Fatalf("Run(thread component bind idempotent) error = %v", err)
	}
	assertThreadComponentMapping(t, ctx, storage, thread.ID, gmail.ID, "bart@example.com")
}

func TestThreadComponentBindErrorsWhenCurrentThreadComponentHasDifferentProviderThreadID(t *testing.T) {
	ctx := context.Background()
	storage, thread := testMessagingStorage(t, ctx)
	gmail := testRegisterComponent(t, ctx, storage, "gmail", "personal")
	if err := storage.ThreadComponentMappings().Save(ctx, &coremodel.ThreadComponentMapping{
		ThreadID:          thread.ID,
		ChatID:            thread.ChatID,
		ComponentID:       gmail.ID,
		ComponentThreadID: "old-provider-thread",
	}); err != nil {
		t.Fatalf("Save(mapping) error = %v", err)
	}
	engine := testMessagingEngine(t, storage)

	_, err := engine.Run(ctx, testMessagingRequest(thread.ID, simplerbac.RoleRoot), []string{"thread", "component", "bind", "gmail/personal", "new-provider-thread"})
	if err == nil || !strings.Contains(err.Error(), "already bound on this thread to provider thread") || !strings.Contains(err.Error(), "old-provider-thread") {
		t.Fatalf("Run(thread component bind current conflict) error = %v, want current-thread conflict", err)
	}
	assertThreadComponentMapping(t, ctx, storage, thread.ID, gmail.ID, "old-provider-thread")
}

func TestThreadComponentBindErrorsWhenProviderThreadIDBelongsToAnotherThread(t *testing.T) {
	ctx := context.Background()
	storage, thread := testMessagingStorage(t, ctx)
	gmail := testRegisterComponent(t, ctx, storage, "gmail", "personal")
	otherThread := &coremodel.Thread{ChatID: thread.ChatID, Label: "other"}
	if err := storage.Threads().Save(ctx, otherThread); err != nil {
		t.Fatalf("Save(other thread) error = %v", err)
	}
	if err := storage.ThreadComponentMappings().Save(ctx, &coremodel.ThreadComponentMapping{
		ThreadID:          otherThread.ID,
		ChatID:            thread.ChatID,
		ComponentID:       gmail.ID,
		ComponentThreadID: "bart@example.com",
	}); err != nil {
		t.Fatalf("Save(mapping) error = %v", err)
	}
	engine := testMessagingEngine(t, storage)

	_, err := engine.Run(ctx, testMessagingRequest(thread.ID, simplerbac.RoleRoot), []string{"thread", "component", "bind", "gmail/personal", "bart@example.com"})
	if err == nil || !strings.Contains(err.Error(), "already bound") {
		t.Fatalf("Run(thread component bind conflict) error = %v, want already-bound error", err)
	}
	if !strings.Contains(err.Error(), otherThread.ID.String()) || !strings.Contains(err.Error(), "short_id:") {
		t.Fatalf("conflict error = %v, want existing thread id and short id", err)
	}
}

func TestThreadComponentBindDeniesUser(t *testing.T) {
	ctx := context.Background()
	storage, thread := testMessagingStorage(t, ctx)
	testRegisterComponent(t, ctx, storage, "gmail", "personal")
	engine := testMessagingEngine(t, storage)

	for _, role := range []simplerbac.Role{simplerbac.RoleUser, simplerbac.RoleAgent} {
		_, err := engine.Run(ctx, testMessagingRequest(thread.ID, role), []string{"thread", "component", "bind", "gmail/personal", "bart@example.com"})
		if err == nil || !strings.Contains(err.Error(), "denied") {
			t.Fatalf("Run(thread component bind as %s) error = %v, want denied", role, err)
		}
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
		ChatID:            chat.ID,
		ComponentID:       telegram.ID,
		Role:              coremodel.ChatComponentRoleSource,
		ExternalChannelID: "-100",
		Enabled:           true,
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

func testRegisterComponent(t *testing.T, ctx context.Context, storage repository.Storage, componentType string, name string) *coremodel.Component {
	t.Helper()
	registration := &coremodel.Component{Type: componentType, Name: name, Enabled: true}
	if err := storage.Components().Save(ctx, registration); err != nil {
		t.Fatalf("Save(component %s/%s) error = %v", componentType, name, err)
	}
	return registration
}

func testSaveChatComponent(t *testing.T, ctx context.Context, storage repository.Storage, binding coremodel.ChatComponent) {
	t.Helper()
	if err := storage.ChatComponents().Save(ctx, &binding); err != nil {
		t.Fatalf("Save(chat component) error = %v", err)
	}
}

func assertThreadComponentMapping(t *testing.T, ctx context.Context, storage repository.Storage, threadID modeluuid.UUID, componentID modeluuid.UUID, providerThreadID string) {
	t.Helper()
	mapping, err := storage.ThreadComponentMappings().GetByThreadAndComponent(ctx, threadID, componentID)
	if err != nil {
		t.Fatalf("GetByThreadAndComponent() error = %v", err)
	}
	if mapping == nil {
		t.Fatalf("thread component mapping missing for thread=%s component=%s", threadID, componentID)
	}
	if got := strings.TrimSpace(mapping.ComponentThreadID); got != providerThreadID {
		t.Fatalf("ComponentThreadID = %q, want %q", got, providerThreadID)
	}
}

func testMessagingRequest(threadID modeluuid.UUID, roles ...simplerbac.Role) commandengine.Request {
	if len(roles) == 0 {
		roles = []simplerbac.Role{simplerbac.RoleRoot}
	}
	return commandengine.Request{Context: commandengine.Context{
		ThreadID: threadID,
		Actor: commandengine.Actor{
			ID:    "bart",
			Label: "Bart",
			Roles: roles,
		},
	}}
}

func TestThreadConfigSetGetListCurrentThread(t *testing.T) {
	ctx := context.Background()
	storage, thread := testMessagingStorage(t, ctx)
	engine := testMessagingEngine(t, storage)

	result, err := engine.Run(ctx, testMessagingRequest(thread.ID, simplerbac.RoleRoot), []string{"thread", "config", "set", "voice.reply_to_voice_input", "true"})
	if err != nil {
		t.Fatalf("Run(thread config set) error = %v", err)
	}
	if got, want := strings.TrimSpace(result.Text), "thread config set: voice.reply_to_voice_input=true"; got != want {
		t.Fatalf("result = %q, want %q", got, want)
	}

	result, err = engine.Run(ctx, testMessagingRequest(thread.ID, simplerbac.RoleRoot), []string{"thread", "config", "get", "voice.reply_to_voice_input"})
	if err != nil {
		t.Fatalf("Run(thread config get) error = %v", err)
	}
	if got, want := strings.TrimSpace(result.Text), "voice.reply_to_voice_input=true"; got != want {
		t.Fatalf("get result = %q, want %q", got, want)
	}

	result, err = engine.Run(ctx, testMessagingRequest(thread.ID, simplerbac.RoleRoot), []string{"thread", "config", "list"})
	if err != nil {
		t.Fatalf("Run(thread config list) error = %v", err)
	}
	if !strings.Contains(result.Text, "voice.reply_to_voice_input=true") || !strings.Contains(result.Text, "voice.name=") {
		t.Fatalf("list result = %q, want thread config values", result.Text)
	}
}

func TestThreadConfigExplicitThreadUsesSameHandler(t *testing.T) {
	ctx := context.Background()
	storage, thread := testMessagingStorage(t, ctx)
	engine := testMessagingEngine(t, storage)

	if _, err := engine.Run(ctx, testMessagingRequest(thread.ID, simplerbac.RoleRoot), []string{"thread", thread.ID.String(), "config", "set", "voice.name", "F5"}); err != nil {
		t.Fatalf("Run(thread <thread> config set) error = %v", err)
	}
	result, err := engine.Run(ctx, testMessagingRequest(thread.ID, simplerbac.RoleRoot), []string{"thread", "config", "get", "voice.name"})
	if err != nil {
		t.Fatalf("Run(thread config get) error = %v", err)
	}
	if got, want := strings.TrimSpace(result.Text), "voice.name=F5"; got != want {
		t.Fatalf("get result = %q, want %q", got, want)
	}
}
