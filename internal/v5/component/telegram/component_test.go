package telegram

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/dbmodel"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	v5component "github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/go-clistate"
)

type sentMessage struct {
	chatID    int64
	threadID  int
	text      string
	parseMode string
}

type sentDocument struct {
	chatID   int64
	threadID int
	filename string
	caption  string
	content  string
}

type sentPhoto struct {
	chatID   int64
	threadID int
	filename string
	caption  string
	content  string
}

type sentChatAction struct {
	chatID   int64
	threadID int
	action   messenger.ChatAction
}

type fakeTelegramAPI struct {
	mu sync.Mutex

	updates     []dbmodel.TelegramUpdate
	runErr      error
	pollTimeout time.Duration

	messages        []sentMessage
	documents       []sentDocument
	photos          []sentPhoto
	videos          []sentPhoto
	audios          []sentPhoto
	actions         []sentChatAction
	sendMessageErrs []error
	downloads       map[string][]byte
}

func (f *fakeTelegramAPI) Run(ctx context.Context, pollTimeout time.Duration, onUpdate func(context.Context, dbmodel.TelegramUpdate)) error {
	f.mu.Lock()
	f.pollTimeout = pollTimeout
	updates := append([]dbmodel.TelegramUpdate(nil), f.updates...)
	runErr := f.runErr
	f.mu.Unlock()
	for _, update := range updates {
		onUpdate(ctx, update)
	}
	return runErr
}

func (f *fakeTelegramAPI) SendMessage(ctx context.Context, chatID int64, threadID int, text string, parseMode string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, sentMessage{chatID: chatID, threadID: threadID, text: text, parseMode: parseMode})
	if len(f.sendMessageErrs) > 0 {
		err := f.sendMessageErrs[0]
		f.sendMessageErrs = f.sendMessageErrs[1:]
		return err
	}
	return nil
}

func (f *fakeTelegramAPI) SendDocument(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.documents = append(f.documents, sentDocument{chatID: chatID, threadID: threadID, filename: filename, caption: caption, content: string(content)})
	return nil
}

func (f *fakeTelegramAPI) SendPhoto(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.photos = append(f.photos, sentPhoto{chatID: chatID, threadID: threadID, filename: filename, caption: caption, content: string(content)})
	return nil
}

func (f *fakeTelegramAPI) SendVideo(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.videos = append(f.videos, sentPhoto{chatID: chatID, threadID: threadID, filename: filename, caption: caption, content: string(content)})
	return nil
}

func (f *fakeTelegramAPI) SendAudio(ctx context.Context, chatID int64, threadID int, filename string, caption string, content []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audios = append(f.audios, sentPhoto{chatID: chatID, threadID: threadID, filename: filename, caption: caption, content: string(content)})
	return nil
}

func (f *fakeTelegramAPI) SendChatAction(ctx context.Context, chatID int64, threadID int, action messenger.ChatAction) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actions = append(f.actions, sentChatAction{chatID: chatID, threadID: threadID, action: action})
	return nil
}

func (f *fakeTelegramAPI) DownloadFile(ctx context.Context, fileID string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.downloads == nil {
		return nil, fmt.Errorf("missing download: %s", fileID)
	}
	content, ok := f.downloads[fileID]
	if !ok {
		return nil, fmt.Errorf("missing download: %s", fileID)
	}
	return append([]byte(nil), content...), nil
}

func (f *fakeTelegramAPI) messageSnapshot() []sentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sentMessage(nil), f.messages...)
}

func (f *fakeTelegramAPI) actionCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.actions)
}

func newTelegramTestConfig(t *testing.T) (*appstate.Config, *clistate.Store) {
	t.Helper()
	root := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("NewCwd: %v", err)
	}
	return appstate.New(filepath.Join(root, ".ctgbot"), store), store
}

func TestRunInboundEmitsV5EventAndRelaysResponse(t *testing.T) {
	cfg, store := newTelegramTestConfig(t)
	if err := store.PersistInt("telegram.defaults.debounce_ms", 0); err != nil {
		t.Fatalf("PersistInt debounce: %v", err)
	}
	api := &fakeTelegramAPI{updates: []dbmodel.TelegramUpdate{{
		ChatID:    123,
		ChatTitle: "Project chat",
		ThreadID:  4,
		MessageID: 99,
		Text:      " hello ",
		Username:  "bart",
		UserID:    7,
	}}}
	componentID := modeluuid.New()
	c := &Component{componentID: componentID, api: api, cfg: cfg}

	var events []v5component.InboundEvent
	err := c.RunInbound(context.Background(), func(ctx context.Context, event v5component.InboundEvent) error {
		events = append(events, event)
		return c.Send(ctx, messenger.OutboundPayload{
			ProviderChatID:   event.Payload.ProviderChatID,
			ProviderThreadID: event.Payload.ProviderThreadID,
			Text:             messenger.TextMessage{Text: " pong "},
		})
	})
	if err != nil {
		t.Fatalf("RunInbound() error = %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	event := events[0]
	if event.ComponentID != componentID || event.ExternalID != "99" {
		t.Fatalf("event id fields = component %s external %q", event.ComponentID, event.ExternalID)
	}
	if event.Payload.ProviderType != Type || event.Payload.ProviderChatID != "123" || event.Payload.ProviderThreadID != "4" {
		t.Fatalf("payload provider fields = %#v", event.Payload)
	}
	if event.Payload.Actor.ID != "7" || event.Payload.Actor.Label != "@bart" {
		t.Fatalf("payload actor = %#v", event.Payload.Actor)
	}
	messages := api.messageSnapshot()
	if len(messages) != 1 || messages[0].chatID != 123 || messages[0].threadID != 4 || messages[0].text != "pong" {
		t.Fatalf("messages = %#v, want one pong to chat/thread", messages)
	}
}

func TestRunInboundDoesNotReturnEmitError(t *testing.T) {
	cfg, store := newTelegramTestConfig(t)
	if err := store.PersistInt("telegram.defaults.debounce_ms", 0); err != nil {
		t.Fatalf("PersistInt debounce: %v", err)
	}
	api := &fakeTelegramAPI{updates: []dbmodel.TelegramUpdate{{ChatID: 1, ThreadID: 2, MessageID: 3, Text: "hi"}}}
	c := &Component{componentID: modeluuid.New(), api: api, cfg: cfg}

	errBoom := errors.New("boom")
	if err := c.RunInbound(context.Background(), func(ctx context.Context, event v5component.InboundEvent) error { return errBoom }); err != nil {
		t.Fatalf("RunInbound() error = %v", err)
	}
}

func TestInboundPayloadMarksConfiguredOperatorAsRoot(t *testing.T) {
	cfg, store := newTelegramTestConfig(t)
	if err := store.PersistStruct("telegram", map[string]any{"operators": []int64{42}}); err != nil {
		t.Fatalf("PersistStruct telegram: %v", err)
	}
	c := &Component{api: &fakeTelegramAPI{}, cfg: cfg}

	payload, err := c.inboundPayload(context.Background(), dbmodel.TelegramUpdate{
		ChatID:    1,
		ThreadID:  2,
		MessageID: 3,
		Text:      "hi",
		UserID:    42,
	}, "hi")
	if err != nil {
		t.Fatalf("inboundPayload() error = %v", err)
	}
	if !payload.Actor.HasRole(simplerbac.RoleRoot) || !payload.Actor.HasRole(simplerbac.RoleUser) {
		t.Fatalf("roles = %#v, want user+root", payload.Actor.Roles)
	}
}

func TestInboundPayloadDownloadsAttachments(t *testing.T) {
	cfg, _ := newTelegramTestConfig(t)
	api := &fakeTelegramAPI{downloads: map[string][]byte{"file-1": []byte("contents")}}
	c := &Component{api: api, cfg: cfg}

	payload, err := c.inboundPayload(context.Background(), dbmodel.TelegramUpdate{
		ChatID:    1,
		ThreadID:  2,
		MessageID: 3,
		Text:      "see attached",
		Attachments: []dbmodel.TelegramAttachment{{
			Kind:     "document",
			FileID:   "file-1",
			Filename: "report.txt",
		}},
	}, "see attached")
	if err != nil {
		t.Fatalf("inboundPayload() error = %v", err)
	}
	if len(payload.Attachments) != 1 {
		t.Fatalf("len(attachments) = %d, want 1", len(payload.Attachments))
	}
	got := payload.Attachments[0]
	if got.Kind != "document" || got.Filename != "report.txt" || string(got.Content) != "contents" {
		t.Fatalf("attachment = %#v", got)
	}
}

func TestSendIgnoresZeroPayload(t *testing.T) {
	api := &fakeTelegramAPI{}
	c := &Component{api: api}
	if err := c.Send(context.Background(), messenger.OutboundPayload{}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if len(api.messageSnapshot()) != 0 {
		t.Fatalf("messages = %#v, want none", api.messageSnapshot())
	}
}

func TestSendFallsBackFromMarkdownToHTML(t *testing.T) {
	cfg, store := newTelegramTestConfig(t)
	if err := store.PersistString("telegram.defaults.render_format", "markdown"); err != nil {
		t.Fatalf("PersistString render format: %v", err)
	}
	api := &fakeTelegramAPI{sendMessageErrs: []error{fmt.Errorf("Bad Request: can't parse entities")}}
	c := &Component{api: api, cfg: cfg}

	if err := c.Send(context.Background(), messenger.OutboundPayload{
		ProviderChatID:   "123",
		ProviderThreadID: "4",
		Text:             messenger.TextMessage{Text: "*hello*"},
	}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	messages := api.messageSnapshot()
	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d, want 2: %#v", len(messages), messages)
	}
	if messages[0].parseMode != "MarkdownV2" || messages[1].parseMode != "HTML" {
		t.Fatalf("parse modes = %q, %q; want MarkdownV2 then HTML", messages[0].parseMode, messages[1].parseMode)
	}
}

func TestSendMediaImageUsesPhoto(t *testing.T) {
	api := &fakeTelegramAPI{}
	c := &Component{api: api}
	if err := c.Send(context.Background(), messenger.OutboundPayload{
		ProviderChatID:   "123",
		ProviderThreadID: "4",
		Text:             messenger.TextMessage{Text: "caption"},
		Attachments: []messenger.Media{{
			Filename:    "image.png",
			ContentType: "image/png",
			Content:     []byte("png"),
		}},
	}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if len(api.photos) != 1 {
		t.Fatalf("photos = %#v, want one", api.photos)
	}
	if got := api.photos[0]; got.chatID != 123 || got.threadID != 4 || got.filename != "image.png" || got.caption != "caption" || got.content != "png" {
		t.Fatalf("photo = %#v", got)
	}
}

func TestSendTextualSourceWithSyntaxUsesRenderedFence(t *testing.T) {
	api := &fakeTelegramAPI{}
	c := &Component{api: api}
	if err := c.Send(context.Background(), messenger.OutboundPayload{
		ProviderChatID: "123",
		Text:           messenger.TextMessage{Text: "Here"},
		Attachments: []messenger.Media{{
			Filename:    "main.go",
			ContentType: "text/plain",
			Syntax:      "go",
			Content:     []byte("package main\n"),
		}},
	}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	messages := api.messageSnapshot()
	if len(messages) != 1 {
		t.Fatalf("messages = %#v, want one", messages)
	}
	if !strings.Contains(messages[0].text, "package main") {
		t.Fatalf("message text = %q, want inline source body", messages[0].text)
	}
}

func TestStartChatActionSendsAndStopsHeartbeat(t *testing.T) {
	oldInterval := chatActionRefreshInterval
	chatActionRefreshInterval = 10 * time.Millisecond
	defer func() { chatActionRefreshInterval = oldInterval }()

	api := &fakeTelegramAPI{}
	c := &Component{api: api}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, err := c.StartChatAction(ctx, messenger.ChatTarget{ProviderChatID: "123", ProviderThreadID: "4"}, messenger.ChatActionTyping)
	if err != nil {
		t.Fatalf("StartChatAction() error = %v", err)
	}
	waitForCondition(t, time.Second, func() bool { return api.actionCount() >= 2 })
	stop()
	afterStop := api.actionCount()
	time.Sleep(3 * chatActionRefreshInterval)
	if got := api.actionCount(); got != afterStop {
		t.Fatalf("action count after stop = %d, want %d", got, afterStop)
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}
