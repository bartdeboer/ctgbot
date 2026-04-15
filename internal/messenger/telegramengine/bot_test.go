package telegramengine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/agent"
	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/chatbroker"
	"github.com/bartdeboer/ctgbot/internal/chatmodel"
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
	chatID   int64
	threadID int
	replyTo  int
	text     string
}

type sentDocument struct {
	chatID   int64
	threadID int
	filename string
	caption  string
	content  []byte
}

type fakeTelegramAPI struct {
	messages  []sentMessage
	documents []sentDocument
	downloads map[string][]byte
}

func (f *fakeTelegramAPI) Run(ctx context.Context, _ time.Duration, _ func(context.Context, chatmodel.TelegramUpdate)) error {
	return nil
}

func (f *fakeTelegramAPI) SendMessage(ctx context.Context, chatID int64, threadID int, replyTo int, text string) error {
	f.messages = append(f.messages, sentMessage{
		chatID:   chatID,
		threadID: threadID,
		replyTo:  replyTo,
		text:     text,
	})
	return nil
}

func (f *fakeTelegramAPI) SendDocument(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error {
	f.documents = append(f.documents, sentDocument{
		chatID:   chatID,
		threadID: threadID,
		filename: filename,
		caption:  caption,
		content:  append([]byte(nil), content...),
	})
	return nil
}

func (f *fakeTelegramAPI) DownloadFile(ctx context.Context, fileID string) ([]byte, error) {
	if data, ok := f.downloads[fileID]; ok {
		return append([]byte(nil), data...), nil
	}
	return nil, fmt.Errorf("file not found: %s", fileID)
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
	sentPrompt  string
	setupCalled bool
}

func (f *fakeAgent) Name() string { return "codex" }

func (f *fakeAgent) SetupEnvironment(ctx context.Context, sbx *sandboxengine.Sandbox) error {
	f.setupCalled = true
	return nil
}

func (f *fakeAgent) HandleTurn(ctx context.Context, sbx *sandboxengine.Sandbox, providerThreadID string, prompt string) (agent.TurnResult, error) {
	f.sentPrompt = prompt
	return agent.TurnResult{Reply: "reply text"}, nil
}

type fakeSandboxManager struct{}

func (f fakeSandboxManager) NewSandbox(name string) *sandboxengine.Sandbox {
	return &sandboxengine.Sandbox{Name: name}
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

	u := chatmodel.TelegramUpdate{
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

	u := chatmodel.TelegramUpdate{
		ChatID:    42,
		ThreadID:  7,
		MessageID: 99,
		Attachments: []chatmodel.TelegramAttachment{{
			Kind:     "document",
			FileID:   "doc-1",
			Filename: "poem.zip",
		}},
	}

	if err := tb.handleUpdateSerialized(context.Background(), u, "", broker.HandleIncomingUpdate); err != nil {
		t.Fatalf("handleUpdateSerialized returned error: %v", err)
	}

	inboxPath := filepath.Join(cfg.ChatWorkspaceDirByID(entry.ID), "inbox", "poem.zip")
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

	u := chatmodel.TelegramUpdate{
		ChatID:    42,
		ThreadID:  7,
		MessageID: 99,
		Attachments: []chatmodel.TelegramAttachment{{
			Kind:     "document",
			FileID:   "doc-2",
			Filename: "notes.txt",
		}},
	}

	if err := tb.handleUpdateSerialized(context.Background(), u, "please review it", broker.HandleIncomingUpdate); err != nil {
		t.Fatalf("handleUpdateSerialized returned error: %v", err)
	}

	if agent.sentPrompt != "please review it" {
		t.Fatalf("sent prompt = %q, want %q", agent.sentPrompt, "please review it")
	}
	inboxPath := filepath.Join(cfg.ChatWorkspaceDirByID(entry.ID), "inbox", "notes.txt")
	if _, err := os.Stat(inboxPath); err != nil {
		t.Fatalf("stat saved upload: %v", err)
	}
	if len(api.messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(api.messages))
	}
	if api.messages[0].text != "upload saved: /workspace/inbox/notes.txt" {
		t.Fatalf("unexpected first message: %q", api.messages[0].text)
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

	u := chatmodel.TelegramUpdate{
		ChatID:    42,
		ThreadID:  7,
		MessageID: 101,
		Attachments: []chatmodel.TelegramAttachment{{
			Kind:     "photo",
			FileID:   "photo-1",
			Filename: "photo-101.jpg",
		}},
	}

	if err := tb.handleUpdateSerialized(context.Background(), u, "please inspect", broker.HandleIncomingUpdate); err != nil {
		t.Fatalf("handleUpdateSerialized returned error: %v", err)
	}

	if agent.sentPrompt != "please inspect" {
		t.Fatalf("sent prompt = %q, want %q", agent.sentPrompt, "please inspect")
	}
	inboxPath := filepath.Join(cfg.ChatWorkspaceDirByID(entry.ID), "inbox", "photo-101.jpg")
	data, err := os.ReadFile(inboxPath)
	if err != nil {
		t.Fatalf("read saved upload: %v", err)
	}
	if string(data) != "jpeg-bytes" {
		t.Fatalf("saved upload contents = %q", string(data))
	}
	if len(api.messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(api.messages))
	}
	if api.messages[0].text != "upload saved: /workspace/inbox/photo-101.jpg" {
		t.Fatalf("unexpected first message: %q", api.messages[0].text)
	}
}
