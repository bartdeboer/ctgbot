package telegramengine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/agent"
	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/chatbroker"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/go-clistate"
)

func ensureTelegramChat(t *testing.T, cfg *appstate.Config, providerChatID int64, title string, enabled bool) *appstate.ChatConfigEntry {
	t.Helper()

	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	entry, err := cfg.EnsureProviderChat("telegram", strconv.FormatInt(providerChatID, 10), title)
	if err != nil {
		t.Fatalf("ensure provider chat: %v", err)
	}
	if enabled {
		if err := cfg.SetChatEnabledByID(entry.ID, true); err != nil {
			t.Fatalf("set chat enabled: %v", err)
		}
	}
	return entry
}

type sentMessage struct {
	chatID    int64
	threadID  int
	replyTo   int
	text      string
	parseMode string
}

type sentDocument struct {
	chatID   int64
	threadID int
	filename string
	caption  string
	content  []byte
}

type sentChatAction struct {
	chatID   int64
	threadID int
	action   messenger.ChatAction
}

type fakeTelegramAPI struct {
	messages        []sentMessage
	documents       []sentDocument
	photos          []sentDocument
	videos          []sentDocument
	audios          []sentDocument
	actions         []sentChatAction
	downloads       map[string][]byte
	failByParseMode map[string]error
	sendHook        func(sentMessage) error
}

func (f *fakeTelegramAPI) Run(ctx context.Context, _ time.Duration, _ func(context.Context, TelegramUpdate)) error {
	return nil
}

func (f *fakeTelegramAPI) SendMessage(ctx context.Context, chatID int64, threadID int, replyTo int, text string, parseMode string) error {
	msg := sentMessage{chatID: chatID, threadID: threadID, replyTo: replyTo, text: text, parseMode: parseMode}
	if f.sendHook != nil {
		if err := f.sendHook(msg); err != nil {
			return err
		}
	}
	if err, ok := f.failByParseMode[parseMode]; ok {
		return err
	}
	f.messages = append(f.messages, msg)
	return nil
	return nil
}

func (f *fakeTelegramAPI) SendDocument(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error {
	f.documents = append(f.documents, sentDocument{chatID: chatID, threadID: threadID, filename: filename, caption: caption, content: append([]byte(nil), content...)})
	return nil
}

func (f *fakeTelegramAPI) SendPhoto(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error {
	f.photos = append(f.photos, sentDocument{chatID: chatID, threadID: threadID, filename: filename, caption: caption, content: append([]byte(nil), content...)})
	return nil
}

func (f *fakeTelegramAPI) SendVideo(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error {
	f.videos = append(f.videos, sentDocument{chatID: chatID, threadID: threadID, filename: filename, caption: caption, content: append([]byte(nil), content...)})
	return nil
}

func (f *fakeTelegramAPI) SendAudio(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error {
	f.audios = append(f.audios, sentDocument{chatID: chatID, threadID: threadID, filename: filename, caption: caption, content: append([]byte(nil), content...)})
	return nil
}

func (f *fakeTelegramAPI) SendChatAction(ctx context.Context, chatID int64, threadID int, action messenger.ChatAction) error {
	f.actions = append(f.actions, sentChatAction{
		chatID:   chatID,
		threadID: threadID,
		action:   action,
	})
	return nil
}

func (f *fakeTelegramAPI) DownloadFile(ctx context.Context, fileID string) ([]byte, error) {
	if data, ok := f.downloads[fileID]; ok {
		return append([]byte(nil), data...), nil
	}
	return nil, fmt.Errorf("file not found: %s", fileID)
}

func TestStartChatActionSendsAndStopsHeartbeat(t *testing.T) {
	prevInterval := chatActionRefreshInterval
	chatActionRefreshInterval = 10 * time.Millisecond
	t.Cleanup(func() {
		chatActionRefreshInterval = prevInterval
	})

	api := &fakeTelegramAPI{}
	tb := &TelegramBot{API: api}

	stop, err := tb.StartChatAction(context.Background(), messenger.ChatTarget{
		ProviderChatID:   "42",
		ProviderThreadID: "7",
	}, messenger.ChatActionTyping)
	if err != nil {
		t.Fatalf("StartChatAction: %v", err)
	}
	time.Sleep(25 * time.Millisecond)
	stop()
	count := len(api.actions)
	if count < 2 {
		t.Fatalf("expected repeated chat actions, got %d", count)
	}
	for _, action := range api.actions {
		if action.chatID != 42 || action.threadID != 7 || action.action != messenger.ChatActionTyping {
			t.Fatalf("unexpected action: %+v", action)
		}
	}
}

type fakeSessionStore struct {
	thread *chatbroker.Thread
}

func (f *fakeSessionStore) AutoMigrate(ctx context.Context) error { return nil }

func (f *fakeSessionStore) FindThread(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*chatbroker.Thread, error) {
	return f.thread, nil
}

func (f *fakeSessionStore) FindThreadByID(ctx context.Context, threadID modeluuid.UUID) (*chatbroker.Thread, error) {
	if f.thread != nil && f.thread.ID == threadID {
		return f.thread, nil
	}
	return nil, nil
}

func (f *fakeSessionStore) EnsureThread(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*chatbroker.Thread, error) {
	if f.thread == nil {
		f.thread = &chatbroker.Thread{ID: modeluuid.New(), ChatID: chatID, ProviderThreadID: providerThreadID}
	}
	return f.thread, nil
}

func (f *fakeSessionStore) SaveThread(ctx context.Context, thread *chatbroker.Thread) error {
	f.thread = thread
	return nil
}

type fakeAgent struct {
	mu          sync.Mutex
	sentPrompt  string
	prompts     []string
	setupCalled bool
}

func (f *fakeAgent) Name() string { return "codex" }

func (f *fakeAgent) SetupEnvironment(ctx context.Context, sbx *sandboxengine.Sandbox) error {
	f.mu.Lock()
	f.setupCalled = true
	f.mu.Unlock()
	return nil
}

func (f *fakeAgent) HandleTurn(ctx context.Context, sbx *sandboxengine.Sandbox, providerThreadID string, prompt string) (agent.TurnResult, error) {
	f.mu.Lock()
	f.sentPrompt = prompt
	f.prompts = append(f.prompts, prompt)
	f.mu.Unlock()
	return agent.TurnResult{Reply: "reply text"}, nil
}

func (f *fakeAgent) PromptCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.prompts)
}

func (f *fakeAgent) PromptSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.prompts))
	copy(out, f.prompts)
	return out
}

type fakeSandboxManager struct{}

func (f fakeSandboxManager) CreateSandbox(spec *sandboxengine.SandboxSpec) *sandboxengine.Sandbox {
	if spec == nil {
		spec = &sandboxengine.SandboxSpec{}
	}
	return &sandboxengine.Sandbox{SandboxSpec: *spec}
}

func TestHandleUpdateSerializedAutoStartsConversation(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	ensureTelegramChat(t, cfg, 42, "Test Chat", true)

	api := &fakeTelegramAPI{}
	sessions := &fakeSessionStore{}
	agent := &fakeAgent{}
	broker := chatbroker.New(cfg, sessions, fakeSandboxManager{}, nil)
	broker.RegisterAgent("codex", agent)
	tb := &TelegramBot{
		API:    api,
		Config: cfg,
	}

	u := TelegramUpdate{
		ChatID:    42,
		ThreadID:  7,
		MessageID: 99,
	}

	if err := tb.handleUpdateSerialized(context.Background(), u, "hello there", broker.HandleIncomingUpdate); err != nil {
		t.Fatalf("handleUpdateSerialized returned error: %v", err)
	}

	if agent.sentPrompt != "hello there" {
		t.Fatalf("sent prompt = %q, want %q", agent.sentPrompt, "hello there")
	}
	if sessions.thread == nil {
		t.Fatalf("expected thread to be created")
	}
	chatCfg, err := cfg.FindProviderChat("telegram", "42")
	if err != nil {
		t.Fatalf("find provider chat: %v", err)
	}
	if chatCfg == nil {
		t.Fatalf("expected provider chat mapping to be created")
	}
	if !sessions.thread.Active {
		t.Fatalf("expected thread to be active")
	}
	if !sessions.thread.Initialized {
		t.Fatalf("expected thread to be initialized")
	}
	if sessions.thread.ChatID != chatCfg.ID {
		t.Fatalf("thread does not reference chat")
	}
	if !agent.setupCalled {
		t.Fatalf("expected environment setup to be called")
	}
	if len(api.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(api.messages))
	}

	wantStart := "conversation started\ncontainer: " + sessions.thread.ContainerName(cfg) + "\nworkspace: " + filepath.Join(root, "chats", chatCfg.ID.String(), "workspace")
	if api.messages[0].text != wantStart {
		t.Fatalf("unexpected first message: %q", api.messages[0].text)
	}
	if api.messages[1].text != "reply text" {
		t.Fatalf("unexpected second message: %q", api.messages[1].text)
	}
}

func TestHandleUpdateSerializedSavesDocumentUpload(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	entry := ensureTelegramChat(t, cfg, 42, "Test Chat", true)

	api := &fakeTelegramAPI{downloads: map[string][]byte{"doc-1": []byte("zip-bytes")}}
	sessions := &fakeSessionStore{}
	broker := chatbroker.New(cfg, sessions, fakeSandboxManager{}, nil)
	tb := &TelegramBot{
		API:    api,
		Config: cfg,
	}

	u := TelegramUpdate{
		ChatID:    42,
		ThreadID:  7,
		MessageID: 99,
		Attachments: []TelegramAttachment{{
			Kind:     "document",
			FileID:   "doc-1",
			Filename: "poem.zip",
		}},
	}

	if err := tb.handleUpdateSerialized(context.Background(), u, "", broker.HandleIncomingUpdate); err != nil {
		t.Fatalf("handleUpdateSerialized returned error: %v", err)
	}

	inboxPath := filepath.Join(cfg.DefaultChatWorkspaceDirByID(entry.ID), "inbox", "poem.zip")
	data, err := os.ReadFile(inboxPath)
	if err != nil {
		t.Fatalf("read saved upload: %v", err)
	}
	if string(data) != "zip-bytes" {
		t.Fatalf("saved upload contents = %q", string(data))
	}
	if len(api.messages) != 1 {
		t.Fatalf("expected 1 confirmation message, got %d", len(api.messages))
	}
	if api.messages[0].text != "upload saved: /workspace/inbox/poem.zip" {
		t.Fatalf("unexpected upload confirmation: %q", api.messages[0].text)
	}
}

func TestHandleUpdateSerializedProcessesTextAfterSavingDocument(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	entry := ensureTelegramChat(t, cfg, 42, "Test Chat", true)

	api := &fakeTelegramAPI{downloads: map[string][]byte{"doc-2": []byte("zip-bytes")}}
	sessions := &fakeSessionStore{}
	agent := &fakeAgent{}
	broker := chatbroker.New(cfg, sessions, fakeSandboxManager{}, nil)
	broker.RegisterAgent("codex", agent)
	tb := &TelegramBot{
		API:    api,
		Config: cfg,
	}

	u := TelegramUpdate{
		ChatID:    42,
		ThreadID:  7,
		MessageID: 99,
		Attachments: []TelegramAttachment{{
			Kind:     "document",
			FileID:   "doc-2",
			Filename: "notes.txt",
		}},
	}

	if err := tb.handleUpdateSerialized(context.Background(), u, "please review it", broker.HandleIncomingUpdate); err != nil {
		t.Fatalf("handleUpdateSerialized returned error: %v", err)
	}

	if agent.sentPrompt != "Files made available:\n- /workspace/inbox/notes.txt\n\nplease review it" {
		t.Fatalf("sent prompt = %q, want %q", agent.sentPrompt, "Files made available:\n- /workspace/inbox/notes.txt\n\nplease review it")
	}
	inboxPath := filepath.Join(cfg.DefaultChatWorkspaceDirByID(entry.ID), "inbox", "notes.txt")
	if _, err := os.Stat(inboxPath); err != nil {
		t.Fatalf("stat saved upload: %v", err)
	}
	if len(api.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(api.messages))
	}
}

func TestHandleUpdateSerializedSavesPhotoUploadAndUsesCaptionAsText(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	entry := ensureTelegramChat(t, cfg, 42, "Test Chat", true)

	api := &fakeTelegramAPI{downloads: map[string][]byte{"photo-1": []byte("jpeg-bytes")}}
	sessions := &fakeSessionStore{}
	agent := &fakeAgent{}
	broker := chatbroker.New(cfg, sessions, fakeSandboxManager{}, nil)
	broker.RegisterAgent("codex", agent)
	tb := &TelegramBot{
		API:    api,
		Config: cfg,
	}

	u := TelegramUpdate{
		ChatID:    42,
		ThreadID:  7,
		MessageID: 101,
		Attachments: []TelegramAttachment{{
			Kind:     "photo",
			FileID:   "photo-1",
			Filename: "photo-101.jpg",
		}},
	}

	if err := tb.handleUpdateSerialized(context.Background(), u, "please inspect", broker.HandleIncomingUpdate); err != nil {
		t.Fatalf("handleUpdateSerialized returned error: %v", err)
	}

	if agent.sentPrompt != "Files made available:\n- /workspace/inbox/photo-101.jpg\n\nplease inspect" {
		t.Fatalf("sent prompt = %q, want %q", agent.sentPrompt, "Files made available:\n- /workspace/inbox/photo-101.jpg\n\nplease inspect")
	}
	inboxPath := filepath.Join(cfg.DefaultChatWorkspaceDirByID(entry.ID), "inbox", "photo-101.jpg")
	data, err := os.ReadFile(inboxPath)
	if err != nil {
		t.Fatalf("read saved upload: %v", err)
	}
	if string(data) != "jpeg-bytes" {
		t.Fatalf("saved upload contents = %q", string(data))
	}
	if len(api.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(api.messages))
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if fn() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("condition not met within %v", timeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestHandleUpdateDebouncesSlidingPrompt(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.PersistInt("telegram.defaults.debounce_ms", 40); err != nil {
		t.Fatalf("persist debounce: %v", err)
	}
	cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	ensureTelegramChat(t, cfg, 42, "Test Chat", true)

	api := &fakeTelegramAPI{}
	sessions := &fakeSessionStore{}
	agent := &fakeAgent{}
	broker := chatbroker.New(cfg, sessions, fakeSandboxManager{}, nil)
	broker.RegisterAgent("codex", agent)
	tb := &TelegramBot{API: api, Config: cfg}
	debouncer := NewDebouncer(cfg.TelegramDebounceWindow(), nil, func(ctx context.Context, u TelegramUpdate) {
		tb.handleUpdate(ctx, u, broker.HandleIncomingUpdate)
	})

	debouncer.HandleUpdate(context.Background(), TelegramUpdate{
		ChatID: 42, ThreadID: 7, MessageID: 100, UserID: 1, Text: "hello",
	})

	time.Sleep(25 * time.Millisecond)
	debouncer.HandleUpdate(context.Background(), TelegramUpdate{
		ChatID: 42, ThreadID: 7, MessageID: 101, UserID: 1, Text: "world",
	})

	time.Sleep(25 * time.Millisecond)
	if got := agent.PromptCount(); got != 0 {
		t.Fatalf("prompt count after timer reset = %d, want 0", got)
	}

	waitForCondition(t, time.Second, func() bool { return agent.PromptCount() == 1 })
	prompts := agent.PromptSnapshot()
	if len(prompts) != 1 || prompts[0] != "hello\n\nworld" {
		t.Fatalf("prompts = %#v, want merged prompt", prompts)
	}
}

func TestHandleUpdateDebounceSeparatesUsers(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.PersistInt("telegram.defaults.debounce_ms", 20); err != nil {
		t.Fatalf("persist debounce: %v", err)
	}
	cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	ensureTelegramChat(t, cfg, 42, "Test Chat", true)

	api := &fakeTelegramAPI{}
	sessions := &fakeSessionStore{}
	agent := &fakeAgent{}
	broker := chatbroker.New(cfg, sessions, fakeSandboxManager{}, nil)
	broker.RegisterAgent("codex", agent)
	tb := &TelegramBot{API: api, Config: cfg}
	debouncer := NewDebouncer(cfg.TelegramDebounceWindow(), nil, func(ctx context.Context, u TelegramUpdate) {
		tb.handleUpdate(ctx, u, broker.HandleIncomingUpdate)
	})

	debouncer.HandleUpdate(context.Background(), TelegramUpdate{
		ChatID: 42, ThreadID: 7, MessageID: 100, UserID: 1, Text: "from alice",
	})
	debouncer.HandleUpdate(context.Background(), TelegramUpdate{
		ChatID: 42, ThreadID: 7, MessageID: 101, UserID: 2, Text: "from bob",
	})

	waitForCondition(t, time.Second, func() bool { return agent.PromptCount() == 2 })
	prompts := agent.PromptSnapshot()
	seen := map[string]bool{}
	for _, prompt := range prompts {
		seen[prompt] = true
	}
	if !seen["from alice"] || !seen["from bob"] {
		t.Fatalf("prompts = %#v, want separate per-user prompts", prompts)
	}
}

func TestHandleUpdateCommandFlushesPendingDebounce(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.PersistInt("telegram.defaults.debounce_ms", 200); err != nil {
		t.Fatalf("persist debounce: %v", err)
	}
	cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	ensureTelegramChat(t, cfg, 42, "Test Chat", true)

	api := &fakeTelegramAPI{}
	sessions := &fakeSessionStore{}
	agent := &fakeAgent{}
	broker := chatbroker.New(cfg, sessions, fakeSandboxManager{}, nil)
	broker.RegisterAgent("codex", agent)
	tb := &TelegramBot{API: api, Config: cfg}
	debouncer := NewDebouncer(cfg.TelegramDebounceWindow(), nil, func(ctx context.Context, u TelegramUpdate) {
		tb.handleUpdate(ctx, u, broker.HandleIncomingUpdate)
	})

	debouncer.HandleUpdate(context.Background(), TelegramUpdate{
		ChatID: 42, ThreadID: 7, MessageID: 100, UserID: 1, Text: "please review",
	})
	if got := agent.PromptCount(); got != 0 {
		t.Fatalf("prompt count before command = %d, want 0", got)
	}

	debouncer.HandleUpdate(context.Background(), TelegramUpdate{
		ChatID: 42, ThreadID: 7, MessageID: 101, UserID: 1, Text: "/help",
	})

	waitForCondition(t, time.Second, func() bool { return agent.PromptCount() == 1 })
	prompts := agent.PromptSnapshot()
	if len(prompts) != 1 || prompts[0] != "please review" {
		t.Fatalf("prompts = %#v, want flushed prompt before command", prompts)
	}
	if len(api.messages) == 0 || !strings.HasPrefix(api.messages[len(api.messages)-1].text, "Commands:\n") || !strings.Contains(api.messages[len(api.messages)-1].text, "/container refresh") {
		t.Fatalf("last message = %#v, want help text", api.messages)
	}
}

func TestTelegramBotSendAgentResponseUsesConfiguredMarkdownRenderMode(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.PersistString("telegram.defaults.render_format", "markdown"); err != nil {
		t.Fatalf("persist render format: %v", err)
	}
	cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	api := &fakeTelegramAPI{}
	tb := &TelegramBot{API: api, Config: cfg}
	if err := tb.SendAgentResponse(context.Background(), messenger.ResolvedOutgoingMessage{
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             "Use **bold** and `code`.",
	}); err != nil {
		t.Fatalf("SendAgentResponse: %v", err)
	}
	if len(api.messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(api.messages))
	}
	if api.messages[0].parseMode != "MarkdownV2" {
		t.Fatalf("parse mode = %q, want %q", api.messages[0].parseMode, "MarkdownV2")
	}
	if api.messages[0].text != "Use *bold* and `code`\\." {
		t.Fatalf("message text = %q", api.messages[0].text)
	}
}

func TestTelegramBotSendAgentResponseFallsBackFromMarkdownToHTML(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.PersistString("telegram.defaults.render_format", "markdown"); err != nil {
		t.Fatalf("persist render format: %v", err)
	}
	cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	api := &fakeTelegramAPI{failByParseMode: map[string]error{"MarkdownV2": fmt.Errorf("can't parse entities")}}
	tb := &TelegramBot{API: api, Config: cfg}
	if err := tb.SendAgentResponse(context.Background(), messenger.ResolvedOutgoingMessage{
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             "Use **bold** and `code`.",
	}); err != nil {
		t.Fatalf("SendAgentResponse: %v", err)
	}
	if len(api.messages) != 1 {
		t.Fatalf("messages len = %d, want 1 successful send", len(api.messages))
	}
	if api.messages[0].parseMode != "HTML" {
		t.Fatalf("parse mode = %q, want HTML", api.messages[0].parseMode)
	}
	if api.messages[0].text != "Use <b>bold</b> and <code>code</code>." {
		t.Fatalf("message text = %q", api.messages[0].text)
	}
}

func TestTelegramBotSendAgentResponseFallsBackFromHTMLToPlain(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWD) })

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.PersistString("telegram.defaults.render_format", "html"); err != nil {
		t.Fatalf("persist render format: %v", err)
	}
	cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	api := &fakeTelegramAPI{failByParseMode: map[string]error{"HTML": fmt.Errorf("unsupported start tag")}}
	tb := &TelegramBot{API: api, Config: cfg}
	if err := tb.SendAgentResponse(context.Background(), messenger.ResolvedOutgoingMessage{
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             "Use **bold** and `code`.",
	}); err != nil {
		t.Fatalf("SendAgentResponse: %v", err)
	}
	if len(api.messages) != 1 {
		t.Fatalf("messages len = %d, want 1 successful send", len(api.messages))
	}
	if api.messages[0].parseMode != "" {
		t.Fatalf("parse mode = %q, want plain", api.messages[0].parseMode)
	}
	if api.messages[0].text != "Use **bold** and `code`." {
		t.Fatalf("message text = %q", api.messages[0].text)
	}
}

func TestTelegramBotSendAgentResponseFallsBackOnlyFailedChunk(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWD) })

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.PersistString("telegram.defaults.render_format", "markdown"); err != nil {
		t.Fatalf("persist render format: %v", err)
	}
	cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	api := &fakeTelegramAPI{sendHook: func(msg sentMessage) error {
		if msg.parseMode == "MarkdownV2" && strings.Contains(msg.text, "second chunk") {
			return fmt.Errorf("can't parse entities")
		}
		return nil
	}}
	tb := &TelegramBot{API: api, Config: cfg}
	text := strings.Repeat("first chunk line\n", 120) + "\n" + strings.Repeat("second chunk line\n", 120)
	if err := tb.SendAgentResponse(context.Background(), messenger.ResolvedOutgoingMessage{
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             text,
	}); err != nil {
		t.Fatalf("SendAgentResponse: %v", err)
	}
	seenMarkdown := 0
	seenHTML := 0
	for _, msg := range api.messages {
		if msg.parseMode == "MarkdownV2" {
			seenMarkdown++
		}
		if msg.parseMode == "HTML" {
			seenHTML++
		}
	}
	if seenHTML == 0 {
		t.Fatalf("messages = %#v, want mixed markdown/html success", api.messages)
	}
}

func TestTelegramBotSendAgentResponseDoesNotFallbackOnNonFormattingError(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWD) })

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.PersistString("telegram.defaults.render_format", "markdown"); err != nil {
		t.Fatalf("persist render format: %v", err)
	}
	cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	api := &fakeTelegramAPI{failByParseMode: map[string]error{"MarkdownV2": fmt.Errorf("network timeout")}}
	tb := &TelegramBot{API: api, Config: cfg}
	err = tb.SendAgentResponse(context.Background(), messenger.ResolvedOutgoingMessage{
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             "Use **bold**.",
	})
	if err == nil {
		t.Fatalf("expected send error")
	}
	if len(api.messages) != 0 {
		t.Fatalf("messages = %#v, want no fallback success messages", api.messages)
	}
}

func TestTelegramBotSendAgentResponseAllowsRenderedTextAboveSemanticChunkLimit(t *testing.T) {
	root := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.PersistString("telegram.defaults.render_format", "markdown"); err != nil {
		t.Fatalf("persist render format: %v", err)
	}
	cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	api := &fakeTelegramAPI{}
	tb := &TelegramBot{API: api, Config: cfg}
	text := strings.Repeat("a", 3190) + strings.Repeat(".", 300)
	if err := tb.SendAgentResponse(context.Background(), messenger.ResolvedOutgoingMessage{
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             text,
	}); err != nil {
		t.Fatalf("SendAgentResponse: %v", err)
	}
	if len(api.messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(api.messages))
	}
	if api.messages[0].parseMode != "MarkdownV2" {
		t.Fatalf("parse mode = %q, want MarkdownV2", api.messages[0].parseMode)
	}
	if n := telegramTextLen(api.messages[0].text); n <= 3500 || n > 4096 {
		t.Fatalf("rendered len = %d, want > 3500 and <= 4096", n)
	}
}

func TestDebouncerDoesNotDuplicateSingleUpdateAttachments(t *testing.T) {
	updates := make(chan TelegramUpdate, 1)
	debouncer := NewDebouncer(10*time.Millisecond, nil, func(ctx context.Context, u TelegramUpdate) {
		updates <- u
	})

	debouncer.HandleUpdate(context.Background(), TelegramUpdate{
		ChatID: 42, ThreadID: 7, MessageID: 1408, UserID: 1,
		Attachments: []TelegramAttachment{{
			Kind:     "photo",
			FileID:   "photo-file-id",
			Filename: "photo-1408.jpg",
		}},
	})

	select {
	case got := <-updates:
		if len(got.Attachments) != 1 {
			t.Fatalf("len(attachments) = %d, want 1", len(got.Attachments))
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for debounced update")
	}
}

func TestSendMediaMarkdownUsesRenderedText(t *testing.T) {
	api := &fakeTelegramAPI{}
	tb := NewTelegramBot(api, nil, nil, nil)
	err := tb.SendMedia(context.Background(), messenger.ResolvedOutgoingMedia{
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Filename:         "note.md",
		ContentType:      "text/markdown",
		Content:          []byte("# Title\n\n- item"),
	})
	if err != nil {
		t.Fatalf("SendMedia: %v", err)
	}
	if len(api.messages) == 0 {
		t.Fatalf("expected markdown file to send message")
	}
	if len(api.documents) != 0 {
		t.Fatalf("did not expect document upload")
	}
}

func TestSendMediaImageUsesPhoto(t *testing.T) {
	api := &fakeTelegramAPI{}
	tb := NewTelegramBot(api, nil, nil, nil)
	err := tb.SendMedia(context.Background(), messenger.ResolvedOutgoingMedia{
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Filename:         "pic.jpg",
		ContentType:      "image/jpeg",
		Content:          []byte("img"),
	})
	if err != nil {
		t.Fatalf("SendMedia: %v", err)
	}
	if len(api.photos) != 1 {
		t.Fatalf("photos = %d, want 1", len(api.photos))
	}
	if len(api.documents) != 0 {
		t.Fatalf("did not expect document upload")
	}
}

func TestSendMediaTextWithSyntaxUsesRenderedFence(t *testing.T) {
	api := &fakeTelegramAPI{}
	tb := NewTelegramBot(api, nil, nil, nil)
	err := tb.SendMedia(context.Background(), messenger.ResolvedOutgoingMedia{
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Filename:         "reply.diff",
		ContentType:      "text/plain",
		Syntax:           "diff",
		Content:          []byte("+hello\n-world\n"),
	})
	if err != nil {
		t.Fatalf("SendMedia: %v", err)
	}
	if len(api.messages) == 0 {
		t.Fatalf("expected inline rendered text")
	}
	if len(api.documents) != 0 {
		t.Fatalf("did not expect document upload")
	}
	if !strings.Contains(api.messages[0].text, "hello") {
		t.Fatalf("message text = %q", api.messages[0].text)
	}
}

func TestSendAgentResponsePlainUsesPlainParseMode(t *testing.T) {
	api := &fakeTelegramAPI{}
	tb := NewTelegramBot(api, nil, nil, nil)
	err := tb.SendAgentResponse(context.Background(), messenger.ResolvedOutgoingMessage{
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             "plain * text",
		ContentType:      "text/plain",
	})
	if err != nil {
		t.Fatalf("SendAgentResponse: %v", err)
	}
	if len(api.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(api.messages))
	}
	if api.messages[0].parseMode != "" {
		t.Fatalf("parseMode = %q, want empty", api.messages[0].parseMode)
	}
	if api.messages[0].text != "plain * text" {
		t.Fatalf("text = %q", api.messages[0].text)
	}
}

func TestSendAgentResponseMarkdownUsesRenderedPath(t *testing.T) {
	api := &fakeTelegramAPI{}
	tb := NewTelegramBot(api, nil, nil, nil)
	err := tb.SendAgentResponse(context.Background(), messenger.ResolvedOutgoingMessage{
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             "# Title\n\n- item",
		ContentType:      "text/markdown",
	})
	if err != nil {
		t.Fatalf("SendAgentResponse: %v", err)
	}
	if len(api.messages) == 0 {
		t.Fatalf("expected rendered markdown messages")
	}
}
