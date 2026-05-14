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

	componentpkg "github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
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
	action   message.ChatAction
}

type fakeTelegramAPI struct {
	mu sync.Mutex

	updates     []TelegramUpdate
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

func (f *fakeTelegramAPI) Run(ctx context.Context, pollTimeout time.Duration, onUpdate func(context.Context, TelegramUpdate)) error {
	f.mu.Lock()
	f.pollTimeout = pollTimeout
	updates := append([]TelegramUpdate(nil), f.updates...)
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

func (f *fakeTelegramAPI) SendChatAction(ctx context.Context, chatID int64, threadID int, action message.ChatAction) error {
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

func telegramTestConfig(config ComponentConfig) ComponentConfig {
	return config.withDefaults()
}

func noDebounceConfig() ComponentConfig {
	return telegramTestConfig(ComponentConfig{DebounceWindow: "0s"})
}

func TestManagedFiles(t *testing.T) {
	c := &Component{}
	files := c.ManagedFiles()
	if len(files) != 2 {
		t.Fatalf("len(ManagedFiles) = %d, want 2", len(files))
	}
	if files[0].RelativePath != TokenFilename || !files[0].Required || !files[0].Sensitive {
		t.Fatalf("token managed file = %#v", files[0])
	}
	if files[1].RelativePath != ComponentConfigFilename || files[1].Required || files[1].Sensitive {
		t.Fatalf("config managed file = %#v", files[1])
	}
}

func TestNewLoadsProfileTokenAndConfig(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, TokenFilename), []byte("123:abc\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	configJSON := `{"operators":[42,42,0],"poll_timeout":"2s","debounce_window":"0s","render_format":"html"}`
	if err := os.WriteFile(filepath.Join(home, ComponentConfigFilename), []byte(configJSON), 0o644); err != nil {
		t.Fatalf("write component config: %v", err)
	}
	loaded, err := New(context.Background(), coremodel.Component{ID: modeluuid.New(), Type: Type, Name: Type}, nil, runtimepkg.Home{Path: home}, repository.NewMemory(), nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	c := loaded.(*Component)
	if c.api == nil {
		t.Fatalf("api is nil")
	}
	if got := c.componentConfig.Operators; len(got) != 1 || got[0] != 42 {
		t.Fatalf("operators = %#v, want [42]", got)
	}
	if got := c.componentConfig.pollTimeout(); got != 2*time.Second {
		t.Fatalf("poll timeout = %s, want 2s", got)
	}
	if got := c.componentConfig.debounceWindow(); got != 0 {
		t.Fatalf("debounce = %s, want 0", got)
	}
	if got := c.componentConfig.renderFormat(); got != "html" {
		t.Fatalf("render format = %q, want html", got)
	}
}

func TestNewAllowsMissingProfileTokenForManagedFileSetup(t *testing.T) {
	loaded, err := New(context.Background(), coremodel.Component{ID: modeluuid.New(), Type: Type, Name: Type}, nil, runtimepkg.Home{Path: t.TempDir()}, repository.NewMemory(), nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	c := loaded.(*Component)
	if c.api != nil {
		t.Fatalf("api = %#v, want nil before token is installed", c.api)
	}
	if got := c.componentConfig.renderFormat(); got != "markdown_v2" {
		t.Fatalf("default render format = %q, want markdown_v2", got)
	}
}

func TestRunInboundWaitsForMissingTokenUntilCancel(t *testing.T) {
	loaded, err := New(context.Background(), coremodel.Component{ID: modeluuid.New(), Type: Type, Name: Type}, nil, runtimepkg.Home{Path: t.TempDir()}, repository.NewMemory(), nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	c := loaded.(*Component)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.RunInbound(ctx, func(context.Context, componentpkg.InboundEvent) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunInbound() error = %v, want context.Canceled", err)
	}
}

func TestRunInboundEmitsInboundEventAndRelaysResponse(t *testing.T) {
	apiConfig := noDebounceConfig()
	api := &fakeTelegramAPI{updates: []TelegramUpdate{{
		ChatID:    123,
		ChatTitle: "Project chat",
		ThreadID:  4,
		MessageID: 99,
		Text:      " hello ",
		Username:  "bart",
		UserID:    7,
	}}}
	componentID := modeluuid.New()
	c := &Component{componentID: componentID, api: api, componentConfig: apiConfig}

	var events []componentpkg.InboundEvent
	err := c.RunInbound(context.Background(), func(ctx context.Context, event componentpkg.InboundEvent) error {
		events = append(events, event)
		return c.Send(ctx, message.OutboundPayload{
			ProviderChannelID: event.Payload.ProviderChannelID,
			ProviderThreadID:  event.Payload.ProviderThreadID,
			Text:              message.TextMessage{Text: " pong "},
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
	if event.Payload.ProviderType != Type || event.Payload.ProviderChannelID != "123" || event.Payload.ProviderThreadID != "4" {
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
	apiConfig := noDebounceConfig()
	api := &fakeTelegramAPI{updates: []TelegramUpdate{{ChatID: 1, ThreadID: 2, MessageID: 3, Text: "hi"}}}
	c := &Component{componentID: modeluuid.New(), api: api, componentConfig: apiConfig}

	errBoom := errors.New("boom")
	if err := c.RunInbound(context.Background(), func(ctx context.Context, event componentpkg.InboundEvent) error { return errBoom }); err != nil {
		t.Fatalf("RunInbound() error = %v", err)
	}
}

func TestInboundPayloadMarksConfiguredOperatorAsRoot(t *testing.T) {
	c := &Component{api: &fakeTelegramAPI{}, componentConfig: telegramTestConfig(ComponentConfig{Operators: []int64{42}})}

	payload, err := c.inboundPayload(context.Background(), TelegramUpdate{
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
	api := &fakeTelegramAPI{downloads: map[string][]byte{"file-1": []byte("contents")}}
	c := &Component{api: api}

	payload, err := c.inboundPayload(context.Background(), TelegramUpdate{
		ChatID:    1,
		ThreadID:  2,
		MessageID: 3,
		Text:      "see attached",
		Attachments: []TelegramAttachment{{
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
	if err := c.Send(context.Background(), message.OutboundPayload{}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if len(api.messageSnapshot()) != 0 {
		t.Fatalf("messages = %#v, want none", api.messageSnapshot())
	}
}

func TestSendUsesMarkdownV2ByDefault(t *testing.T) {
	api := &fakeTelegramAPI{}
	c := &Component{api: api}

	if err := c.Send(context.Background(), message.OutboundPayload{
		ProviderChannelID: "123",
		ProviderThreadID:  "4",
		Text:              message.TextMessage{Text: "*hello*"},
	}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	messages := api.messageSnapshot()
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1: %#v", len(messages), messages)
	}
	if messages[0].parseMode != "MarkdownV2" {
		t.Fatalf("parse mode = %q, want MarkdownV2", messages[0].parseMode)
	}
}

func TestSendDefaultMarkdownV2FallsBackToHTMLThenPlain(t *testing.T) {
	api := &fakeTelegramAPI{sendMessageErrs: []error{fmt.Errorf("Bad Request: can't parse entities"), fmt.Errorf("Bad Request: can't parse entities")}}
	c := &Component{api: api}

	if err := c.Send(context.Background(), message.OutboundPayload{
		ProviderChannelID: "123",
		ProviderThreadID:  "4",
		Text:              message.TextMessage{Text: "*hello*"},
	}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	messages := api.messageSnapshot()
	if len(messages) != 3 {
		t.Fatalf("len(messages) = %d, want 3: %#v", len(messages), messages)
	}
	if messages[0].parseMode != "MarkdownV2" || messages[1].parseMode != "HTML" || messages[2].parseMode != "" {
		t.Fatalf("parse modes = %q, %q, %q; want MarkdownV2 then HTML then plain", messages[0].parseMode, messages[1].parseMode, messages[2].parseMode)
	}
}

func TestSendFallsBackFromMarkdownToHTML(t *testing.T) {
	apiConfig := telegramTestConfig(ComponentConfig{RenderFormat: "markdown"})
	api := &fakeTelegramAPI{sendMessageErrs: []error{fmt.Errorf("Bad Request: can't parse entities")}}
	c := &Component{api: api, componentConfig: apiConfig}

	if err := c.Send(context.Background(), message.OutboundPayload{
		ProviderChannelID: "123",
		ProviderThreadID:  "4",
		Text:              message.TextMessage{Text: "*hello*"},
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

func TestSendTextMessageWithHTMLContentTypeUsesHTMLParseMode(t *testing.T) {
	api := &fakeTelegramAPI{}
	c := &Component{api: api}

	if err := c.Send(context.Background(), message.OutboundPayload{
		ProviderChannelID: "123",
		Text: message.TextMessage{
			Text:        "<b>hello</b>",
			ContentType: "text/html",
		},
	}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	messages := api.messageSnapshot()
	if len(messages) != 1 {
		t.Fatalf("messages = %#v, want one", messages)
	}
	if messages[0].text != "<b>hello</b>" || messages[0].parseMode != "HTML" {
		t.Fatalf("message = %#v, want raw HTML parse mode", messages[0])
	}
}

func TestSendTextMessageWithPlainContentTypeUsesPlainParseMode(t *testing.T) {
	api := &fakeTelegramAPI{}
	c := &Component{api: api}

	if err := c.Send(context.Background(), message.OutboundPayload{
		ProviderChannelID: "123",
		Text: message.TextMessage{
			Text:        "*hello*",
			ContentType: "text/plain",
		},
	}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	messages := api.messageSnapshot()
	if len(messages) != 1 {
		t.Fatalf("messages = %#v, want one", messages)
	}
	if messages[0].text != "*hello*" || messages[0].parseMode != "" {
		t.Fatalf("message = %#v, want plain parse mode", messages[0])
	}
}
func TestSendMediaImageUsesPhoto(t *testing.T) {
	api := &fakeTelegramAPI{}
	c := &Component{api: api}
	if err := c.Send(context.Background(), message.OutboundPayload{
		ProviderChannelID: "123",
		ProviderThreadID:  "4",
		Text:              message.TextMessage{Text: "caption"},
		Attachments: []message.Media{{
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
	if err := c.Send(context.Background(), message.OutboundPayload{
		ProviderChannelID: "123",
		Text:              message.TextMessage{Text: "Here"},
		Attachments: []message.Media{{
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
	stop, err := c.StartChatAction(ctx, message.ChatTarget{ProviderChannelID: "123", ProviderThreadID: "4"}, message.ChatActionTyping)
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
