package chatbroker

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/agent"
	"github.com/bartdeboer/ctgbot/internal/appstate"
	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/go-clistate"
)

func ensureTelegramChat(t *testing.T, cfg *appstate.Config, providerChatID int64, title string, enabled bool, processTools bool) *appstate.ChatConfigEntry {
	t.Helper()

	entry, err := cfg.EnsureProviderChat("telegram", strconv.FormatInt(providerChatID, 10), title)
	if err != nil {
		t.Fatalf("ensure provider chat: %v", err)
	}
	if enabled {
		if err := cfg.Chat(entry.ID).SetEnabled(true); err != nil {
			t.Fatalf("set chat enabled: %v", err)
		}
	}
	if processTools {
		if err := cfg.Chat(entry.ID).SetProcessToolsEnabled(true); err != nil {
			t.Fatalf("set chat process tools enabled: %v", err)
		}
	}
	return entry
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func TestNewSandboxIncludesInternalChatAndThreadIDs(t *testing.T) {
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

	chatID := modeluuid.New()
	threadID := modeluuid.New()
	broker := New(cfg, &fakeBrokerSessionStore{}, fakeBrokerSandboxManager{}, nil)
	sbx := broker.sandboxForThread(&Thread{
		ID:                 threadID,
		ChatID:             chatID,
		RuntimeName:        "ctgbot-test",
		WorkspaceHost:      filepath.Join(root, "workspace"),
		HomeHost:           filepath.Join(root, "home"),
		ContainerWorkspace: "/workspace",
		ContainerHome:      "/home/codex",
	})

	if !containsString(sbx.Env, "CTGBOT_SANDBOX_ID="+threadID.String()) {
		t.Fatalf("expected CTGBOT_SANDBOX_ID in sandbox env: %#v", sbx.Env)
	}
	if sbx.GPUs != "" {
		t.Fatalf("expected GPUs disabled by default, got %q", sbx.GPUs)
	}
	if err := cfg.Chat(chatID).SetGPUs("all"); err != nil {
		t.Fatalf("set chat gpus: %v", err)
	}
	sbx = broker.sandboxForThread(&Thread{
		ID:                 threadID,
		ChatID:             chatID,
		RuntimeName:        "ctgbot-test",
		WorkspaceHost:      filepath.Join(root, "workspace"),
		HomeHost:           filepath.Join(root, "home"),
		ContainerWorkspace: "/workspace",
		ContainerHome:      "/home/codex",
	})
	if sbx.GPUs != "all" {
		t.Fatalf("expected GPUs=all, got %q", sbx.GPUs)
	}
}

func TestHandleInboundPayloadRoutesTelegramCommand(t *testing.T) {
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
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	ensureTelegramChat(t, cfg, 42, "Test Chat", true, false)

	sessions := &fakeBrokerSessionStore{}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)

	result, err := broker.HandleInboundPayload(context.Background(), messenger.InboundPayload{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             messenger.TextMessage{Text: "/help@ctgbot"},
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	for _, want := range []string{"/config list", "/config set <key> <value>", "/refresh", "/container refresh", "/status"} {
		if !strings.Contains(result.Text.Text, want) {
			t.Fatalf("help text missing %q:\n%s", want, result.Text.Text)
		}
	}
	if strings.Contains(result.Text.Text, "/config hostbridge scaffold") {
		t.Fatalf("help text includes CLI-only scaffold command:\n%s", result.Text.Text)
	}
}

func TestHandleInboundPayloadConfigSetUsesElevatedChatRole(t *testing.T) {
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
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	chat := ensureTelegramChat(t, cfg, 42, "Test Chat", true, true)

	broker := New(cfg, &fakeBrokerSessionStore{}, fakeBrokerSandboxManager{}, nil)
	result, err := broker.HandleInboundPayload(context.Background(), messenger.InboundPayload{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             messenger.TextMessage{Text: "/config set chat.enabled false"},
		ChatLabel:        "Test Chat",
		UserLabel:        "member",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if result.Text.Text != "chat.enabled=false" {
		t.Fatalf("reply = %q, want chat.enabled=false", result.Text.Text)
	}
	if cfg.Chat(chat.ID).Enabled() {
		t.Fatal("chat.enabled was not updated through message command")
	}
}

func TestHandleInboundPayloadRejectsHostbridgeScaffoldCommand(t *testing.T) {
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
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	chat := ensureTelegramChat(t, cfg, 42, "Test Chat", true, true)

	broker := New(cfg, &fakeBrokerSessionStore{}, fakeBrokerSandboxManager{}, nil)
	result, err := broker.HandleInboundPayload(context.Background(), messenger.InboundPayload{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             messenger.TextMessage{Text: "/config hostbridge scaffold deploy"},
		ChatLabel:        "Test Chat",
		UserLabel:        "member",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if !strings.Contains(result.Text.Text, "command error: no matching command") {
		t.Fatalf("reply = %q, want command rejection", result.Text.Text)
	}

	var commands map[string]hostbridgeserver.AllowedCommand
	key := `chats["` + chat.ID.String() + `"].hostbridge.allowed_commands`
	if store.GetStruct(key, &commands) {
		t.Fatalf("allowed command skeleton stored at %s: %#v", key, commands)
	}
}

func TestHandleInboundPayloadRunsUpgradeCommand(t *testing.T) {
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
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	ensureTelegramChat(t, cfg, 42, "Test Chat", true, true)

	process := &fakeProcessActions{}
	sessions := &fakeBrokerSessionStore{}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)
	broker.ProcessActions = process

	result, err := broker.HandleInboundPayload(context.Background(), messenger.InboundPayload{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             messenger.TextMessage{Text: "/upgrade"},
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if !process.upgradeCalled {
		t.Fatalf("expected upgrade to be called")
	}
	if result.Text.Text != "upgrade completed\ntype /quit to restart" {
		t.Fatalf("message text = %q", result.Text.Text)
	}
}

func TestHandleInboundPayloadRunsQuitCommand(t *testing.T) {
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
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	ensureTelegramChat(t, cfg, 42, "Test Chat", true, true)

	process := &fakeProcessActions{}
	sessions := &fakeBrokerSessionStore{}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)
	broker.ProcessActions = process

	result, err := broker.HandleInboundPayload(context.Background(), messenger.InboundPayload{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             messenger.TextMessage{Text: "/quit"},
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if !process.quitCalled {
		t.Fatalf("expected quit to be called")
	}
	if result.Text.Text != "shutting down ctgbot" {
		t.Fatalf("message text = %q", result.Text.Text)
	}
}

func TestHandleInboundPayloadStartsTypingChatAction(t *testing.T) {
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
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	ensureTelegramChat(t, cfg, 42, "Test Chat", true, false)

	sessions := &fakeBrokerSessionStore{}
	provider := &fakeOutboundBrokerProvider{}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)
	broker.RegisterAgent("codex", fakeBrokerAgent{})
	broker.RegisterOutboundChatProvider("telegram", provider)

	result, err := broker.HandleInboundPayload(context.Background(), messenger.InboundPayload{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             messenger.TextMessage{Text: "hello"},
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if result.Text.Text == "" {
		t.Fatalf("expected response text")
	}
	if !reflect.DeepEqual(provider.actions, []messenger.ChatAction{messenger.ChatActionTyping}) {
		t.Fatalf("actions = %#v", provider.actions)
	}
	if !reflect.DeepEqual(provider.stoppedActions, []messenger.ChatAction{messenger.ChatActionTyping}) {
		t.Fatalf("stopped actions = %#v", provider.stoppedActions)
	}
	if len(provider.actionTargets) != 1 {
		t.Fatalf("action targets len = %d, want 1", len(provider.actionTargets))
	}
	if provider.actionTargets[0].ProviderChatID != "42" || provider.actionTargets[0].ProviderThreadID != "7" {
		t.Fatalf("unexpected action target: %+v", provider.actionTargets[0])
	}
}

func TestHandleInboundPayloadSendsIntermediateAgentMessages(t *testing.T) {
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
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	ensureTelegramChat(t, cfg, 42, "Test Chat", true, false)

	sessions := &fakeBrokerSessionStore{}
	provider := &fakeOutboundBrokerProvider{}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)
	broker.RegisterAgent("codex", fakeStreamingBrokerAgent{
		intermediate: "checking runtime",
		reply:        "checking runtime",
	})
	broker.RegisterOutboundChatProvider("telegram", provider)

	result, err := broker.HandleInboundPayload(context.Background(), messenger.InboundPayload{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             messenger.TextMessage{Text: "hello"},
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if result.Text.Text != "" {
		t.Fatalf("expected duplicate final response to be suppressed, got %q", result.Text.Text)
	}
	if len(provider.payloads) != 1 {
		t.Fatalf("payloads len = %d, want 1", len(provider.payloads))
	}
	if provider.payloads[0].Text.Text != "checking runtime" {
		t.Fatalf("intermediate text = %q", provider.payloads[0].Text.Text)
	}
}

func TestHandleInboundPayloadBlocksUpgradeWithoutProcessTools(t *testing.T) {
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
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	ensureTelegramChat(t, cfg, 42, "Test Chat", true, false)

	process := &fakeProcessActions{}
	sessions := &fakeBrokerSessionStore{}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)
	broker.ProcessActions = process

	result, err := broker.HandleInboundPayload(context.Background(), messenger.InboundPayload{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             messenger.TextMessage{Text: "/upgrade"},
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if process.upgradeCalled {
		t.Fatalf("did not expect upgrade to be called")
	}
	if result.Text.Text != "upgrade is not enabled for this chat" {
		t.Fatalf("message text = %q", result.Text.Text)
	}
}

func TestHandleInboundPayloadBlocksQuitWithoutProcessTools(t *testing.T) {
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
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	ensureTelegramChat(t, cfg, 42, "Test Chat", true, false)

	process := &fakeProcessActions{}
	sessions := &fakeBrokerSessionStore{}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)
	broker.ProcessActions = process

	result, err := broker.HandleInboundPayload(context.Background(), messenger.InboundPayload{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             messenger.TextMessage{Text: "/quit"},
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if process.quitCalled {
		t.Fatalf("did not expect quit to be called")
	}
	if result.Text.Text != "quit is not enabled for this chat" {
		t.Fatalf("message text = %q", result.Text.Text)
	}
}

func TestHandleInboundPayloadRefreshesActiveConversation(t *testing.T) {
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
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	chatEntry := ensureTelegramChat(t, cfg, 42, "Test Chat", true, false)
	threadID := modeluuid.New()
	thread := &Thread{
		ID:                 threadID,
		ChatID:             chatEntry.ID,
		ProviderThreadID:   "7",
		AgentProviderType:  "codex",
		RuntimeName:        cfg.Thread(modeluuid.Nil, threadID).ContainerName(),
		WorkspaceHost:      filepath.Join(root, "workspace"),
		HomeHost:           cfg.Chat(chatEntry.ID).CodexProfileHostPath(),
		ContainerWorkspace: cfg.Docker().ContainerWorkspacePath(),
		ContainerHome:      cfg.Docker().ContainerHomePath(),
		AgentThreadID:      "agent-thread-123",
		Initialized:        true,
		Active:             true,
		LastError:          "old error",
	}
	if err := os.MkdirAll(thread.WorkspaceHost, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.MkdirAll(thread.HomeHost, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}

	sessions := &fakeBrokerSessionStore{thread: thread}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)
	broker.RegisterAgent("codex", fakeBrokerAgent{})

	result, err := broker.HandleInboundPayload(context.Background(), messenger.InboundPayload{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             messenger.TextMessage{Text: "/refresh"},
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if result.Text.Text != "conversation runtime refreshed" {
		t.Fatalf("message text = %q", result.Text.Text)
	}
	if sessions.thread == nil {
		t.Fatalf("expected saved thread")
	}
	if sessions.thread.Initialized {
		t.Fatalf("expected refreshed thread to be marked uninitialized")
	}
	if sessions.thread.AgentThreadID != "agent-thread-123" {
		t.Fatalf("AgentThreadID = %q, want preserved value", sessions.thread.AgentThreadID)
	}
	if sessions.thread.LastError != "" {
		t.Fatalf("LastError = %q, want cleared", sessions.thread.LastError)
	}
}

func TestHandleInboundPayloadRefreshDefersSkillInstallUntilNextRuntime(t *testing.T) {
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
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	chatEntry := ensureTelegramChat(t, cfg, 42, "Test Chat", true, false)
	skillOne := filepath.Join(root, "skills", "one")
	skillTwo := filepath.Join(root, "skills", "two")
	if err := cfg.Chat(chatEntry.ID).SetSkills([]string{skillTwo, skillOne}); err != nil {
		t.Fatalf("set chat skills: %v", err)
	}

	threadID := modeluuid.New()
	thread := &Thread{
		ID:                 threadID,
		ChatID:             chatEntry.ID,
		ProviderThreadID:   "7",
		AgentProviderType:  "codex",
		RuntimeName:        cfg.Thread(modeluuid.Nil, threadID).ContainerName(),
		WorkspaceHost:      filepath.Join(root, "workspace"),
		HomeHost:           cfg.Chat(chatEntry.ID).CodexProfileHostPath(),
		ContainerWorkspace: cfg.Docker().ContainerWorkspacePath(),
		ContainerHome:      cfg.Docker().ContainerHomePath(),
		AgentThreadID:      "agent-thread-123",
		Initialized:        true,
		Active:             true,
	}
	if err := os.MkdirAll(thread.WorkspaceHost, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.MkdirAll(thread.HomeHost, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}

	sessions := &fakeBrokerSessionStore{thread: thread}
	agent := &fakeSkillInstallingBrokerAgent{}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)
	broker.RegisterAgent("codex", agent)

	result, err := broker.HandleInboundPayload(context.Background(), messenger.InboundPayload{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             messenger.TextMessage{Text: "/refresh"},
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if result.Text.Text != "conversation runtime refreshed" {
		t.Fatalf("unexpected refresh response: %#v", result)
	}
	if len(agent.installedSkills) != 0 {
		t.Fatalf("installedSkills = %#v, want none until next runtime", agent.installedSkills)
	}
	if sessions.thread.Initialized {
		t.Fatalf("expected refreshed thread to be marked uninitialized")
	}
}

func TestHandleInboundPayloadRefreshWithoutActiveConversation(t *testing.T) {
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
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	ensureTelegramChat(t, cfg, 42, "Test Chat", true, false)

	sessions := &fakeBrokerSessionStore{
		thread: &Thread{
			ID:               modeluuid.New(),
			ChatID:           modeluuid.New(),
			ProviderThreadID: "7",
			Active:           false,
		},
	}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)

	result, err := broker.HandleInboundPayload(context.Background(), messenger.InboundPayload{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             messenger.TextMessage{Text: "/refresh"},
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if result.Text.Text != "no active conversation" {
		t.Fatalf("message text = %q", result.Text.Text)
	}
}

func TestHandleInboundPayloadStatusIncludesInternalIDs(t *testing.T) {
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
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	chatCfg := ensureTelegramChat(t, cfg, 42, "Test Chat", true, false)
	threadID := modeluuid.New()

	sessions := &fakeBrokerSessionStore{
		thread: &Thread{
			ID:               threadID,
			ChatID:           chatCfg.ID,
			ProviderThreadID: "7",
			Active:           true,
			Initialized:      true,
			WorkspaceHost:    "/tmp/workspace",
			LastError:        "previous error",
		},
	}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)

	result, err := broker.HandleInboundPayload(context.Background(), messenger.InboundPayload{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             messenger.TextMessage{Text: "/status"},
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	for _, want := range []string{
		"active conversation",
		"chat_id: " + chatCfg.ID.String(),
		"thread_id: " + threadID.String(),
	} {
		if !strings.Contains(result.Text.Text, want) {
			t.Fatalf("status missing %q:\n%s", want, result.Text.Text)
		}
	}
	if strings.Contains(result.Text.Text, "last_error") || strings.Contains(result.Text.Text, "previous error") {
		t.Fatalf("status should not expose last error:\n%s", result.Text.Text)
	}
}

func TestHandleInboundPayloadPurgesActiveConversation(t *testing.T) {
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
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	chatEntry := ensureTelegramChat(t, cfg, 42, "Test Chat", true, false)
	threadID := modeluuid.New()
	thread := &Thread{
		ID:                 threadID,
		ChatID:             chatEntry.ID,
		ProviderThreadID:   "7",
		AgentProviderType:  "codex",
		RuntimeName:        cfg.Thread(modeluuid.Nil, threadID).ContainerName(),
		WorkspaceHost:      filepath.Join(root, "workspace"),
		HomeHost:           cfg.Chat(chatEntry.ID).CodexProfileHostPath(),
		ContainerWorkspace: cfg.Docker().ContainerWorkspacePath(),
		ContainerHome:      cfg.Docker().ContainerHomePath(),
		AgentThreadID:      "agent-thread-123",
		Initialized:        true,
		Active:             true,
		LastError:          "old error",
	}
	if err := os.MkdirAll(thread.WorkspaceHost, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.MkdirAll(thread.HomeHost, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}

	sessions := &fakeBrokerSessionStore{thread: thread}
	agent := &fakePurgingBrokerAgent{}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)
	broker.RegisterAgent("codex", agent)

	result, err := broker.HandleInboundPayload(context.Background(), messenger.InboundPayload{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             messenger.TextMessage{Text: "/purge"},
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if result.Text.Text != "conversation purged" {
		t.Fatalf("message text = %q", result.Text.Text)
	}
	if !agent.purgeCalled {
		t.Fatalf("expected purge hook to be called")
	}
	if agent.providerThreadID != "agent-thread-123" {
		t.Fatalf("providerThreadID = %q", agent.providerThreadID)
	}
	if sessions.thread == nil {
		t.Fatalf("expected saved thread")
	}
	if sessions.thread.Active {
		t.Fatalf("expected purged thread to be inactive")
	}
	if sessions.thread.Initialized {
		t.Fatalf("expected purged thread to be uninitialized")
	}
	if sessions.thread.AgentThreadID != "" {
		t.Fatalf("AgentThreadID = %q, want cleared", sessions.thread.AgentThreadID)
	}
	if sessions.thread.LastError != "" {
		t.Fatalf("LastError = %q, want cleared", sessions.thread.LastError)
	}
}

func TestHandleInboundPayloadPurgeWithoutActiveConversation(t *testing.T) {
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
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	ensureTelegramChat(t, cfg, 42, "Test Chat", true, false)

	sessions := &fakeBrokerSessionStore{
		thread: &Thread{
			ID:               modeluuid.New(),
			ChatID:           modeluuid.New(),
			ProviderThreadID: "7",
			Active:           false,
		},
	}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)

	result, err := broker.HandleInboundPayload(context.Background(), messenger.InboundPayload{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             messenger.TextMessage{Text: "/purge"},
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if result.Text.Text != "no active conversation" {
		t.Fatalf("message text = %q", result.Text.Text)
	}
}

func TestBrokerSendMediaRoutesToOutboundProvider(t *testing.T) {
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
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	chat := ensureTelegramChat(t, cfg, 42, "Test Chat", true, false)

	thread := &Thread{ID: modeluuid.New(), ChatID: chat.ID, ProviderThreadID: "7"}
	sessions := &fakeBrokerSessionStore{thread: thread}
	provider := &fakeOutboundBrokerProvider{}

	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)
	broker.RegisterOutboundChatProvider("telegram", provider)

	err = broker.SendPayload(context.Background(), thread.ID, messenger.OutboundPayload{
		Text: messenger.TextMessage{Text: "Weekly report"},
		Attachments: []messenger.Media{{
			Filename: "report.pdf",
			Syntax:   "pdf",
			Content:  []byte("hello"),
		}},
	})
	if err != nil {
		t.Fatalf("SendPayload: %v", err)
	}
	if provider.payload == nil {
		t.Fatalf("expected outbound provider to receive media")
	}
	if provider.payload.ProviderChatID != "42" || provider.payload.ProviderThreadID != "7" {
		t.Fatalf("unexpected provider target: %+v", *provider.payload)
	}
	if provider.payload.Text.Text != "Weekly report" {
		t.Fatalf("unexpected caption text: %+v", *provider.payload)
	}
	if len(provider.payload.Attachments) != 1 {
		t.Fatalf("attachments len = %d, want 1", len(provider.payload.Attachments))
	}
	attachment := provider.payload.Attachments[0]
	if attachment.Filename != "report.pdf" {
		t.Fatalf("unexpected attachment metadata: %+v", attachment)
	}
	if attachment.Syntax != "pdf" {
		t.Fatalf("unexpected syntax: %q", attachment.Syntax)
	}
	if string(attachment.Content) != "hello" {
		t.Fatalf("unexpected outbound content: %q", string(attachment.Content))
	}
	if !reflect.DeepEqual(provider.actions, []messenger.ChatAction{messenger.ChatActionUploadDocument}) {
		t.Fatalf("actions = %#v", provider.actions)
	}
	if !reflect.DeepEqual(provider.stoppedActions, []messenger.ChatAction{messenger.ChatActionUploadDocument}) {
		t.Fatalf("stopped actions = %#v", provider.stoppedActions)
	}
	if len(provider.actionTargets) != 1 {
		t.Fatalf("action targets len = %d, want 1", len(provider.actionTargets))
	}
	if provider.actionTargets[0].ProviderChatID != "42" || provider.actionTargets[0].ProviderThreadID != "7" {
		t.Fatalf("unexpected action target: %+v", provider.actionTargets[0])
	}
}

type fakeBrokerSessionStore struct {
	thread *Thread
}

func (f *fakeBrokerSessionStore) AutoMigrate(ctx context.Context) error { return nil }

func (f *fakeBrokerSessionStore) FindThread(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*Thread, error) {
	return f.thread, nil
}

func (f *fakeBrokerSessionStore) FindThreadByID(ctx context.Context, threadID modeluuid.UUID) (*Thread, error) {
	if f.thread != nil && f.thread.ID == threadID {
		return f.thread, nil
	}
	return nil, nil
}

func (f *fakeBrokerSessionStore) EnsureThread(ctx context.Context, chatID modeluuid.UUID, providerThreadID string) (*Thread, error) {
	if f.thread == nil {
		f.thread = &Thread{ID: modeluuid.New(), ChatID: chatID, ProviderThreadID: providerThreadID}
	}
	return f.thread, nil
}

func (f *fakeBrokerSessionStore) SaveThread(ctx context.Context, thread *Thread) error {
	f.thread = thread
	return nil
}

type fakeBrokerSandboxManager struct{}

func (f fakeBrokerSandboxManager) CreateSandbox(spec *sandboxengine.SandboxSpec) *sandboxengine.Sandbox {
	if spec == nil {
		spec = &sandboxengine.SandboxSpec{}
	}
	return &sandboxengine.Sandbox{SandboxSpec: *spec}
}

type fakeProcessActions struct {
	upgradeCalled bool
	quitCalled    bool
}

func (f *fakeProcessActions) Upgrade(ctx context.Context) error {
	f.upgradeCalled = true
	return nil
}

func (f *fakeProcessActions) Quit(ctx context.Context) error {
	f.quitCalled = true
	return nil
}

type fakeBrokerAgent struct{}

func (fakeBrokerAgent) Name() string { return "codex" }

func (fakeBrokerAgent) SetupEnvironment(ctx context.Context, sbx *sandboxengine.Sandbox) error {
	return nil
}

func (fakeBrokerAgent) HandleTurn(ctx context.Context, sbx *sandboxengine.Sandbox, output agent.OutputHandler, providerThreadID string, prompt string) (agent.TurnResult, error) {
	return agent.TurnResult{Reply: "reply text"}, nil
}

type fakeStreamingBrokerAgent struct {
	intermediate string
	reply        string
}

func (fakeStreamingBrokerAgent) Name() string { return "codex" }

func (fakeStreamingBrokerAgent) SetupEnvironment(ctx context.Context, sbx *sandboxengine.Sandbox) error {
	return nil
}

func (f fakeStreamingBrokerAgent) HandleTurn(ctx context.Context, sbx *sandboxengine.Sandbox, output agent.OutputHandler, providerThreadID string, prompt string) (agent.TurnResult, error) {
	if output != nil {
		if err := output.Send(ctx, messenger.OutboundPayload{
			Text: messenger.TextMessage{Text: f.intermediate},
		}); err != nil {
			return agent.TurnResult{}, err
		}
	}
	return agent.TurnResult{Reply: f.reply}, nil
}

type fakePurgingBrokerAgent struct {
	fakeBrokerAgent
	purgeCalled      bool
	providerThreadID string
}

func (f *fakePurgingBrokerAgent) Purge(ctx context.Context, sbx *sandboxengine.Sandbox, providerThreadID string) error {
	f.purgeCalled = true
	f.providerThreadID = providerThreadID
	return nil
}

type fakeSkillInstallingBrokerAgent struct {
	fakeBrokerAgent
	installedSkills []string
}

func (f *fakeSkillInstallingBrokerAgent) InstallSkill(ctx context.Context, sbx *sandboxengine.Sandbox, skillDir string) error {
	f.installedSkills = append(f.installedSkills, skillDir)
	return nil
}

type fakeOutboundBrokerProvider struct {
	payload        *messenger.OutboundPayload
	payloads       []messenger.OutboundPayload
	actions        []messenger.ChatAction
	actionTargets  []messenger.ChatTarget
	stoppedActions []messenger.ChatAction
}

func (f *fakeOutboundBrokerProvider) ProviderType() string { return "telegram" }

func (f *fakeOutboundBrokerProvider) Send(ctx context.Context, payload messenger.OutboundPayload) error {
	copyPayload := payload
	copyPayload.Attachments = append([]messenger.Media(nil), payload.Attachments...)
	for i := range copyPayload.Attachments {
		copyPayload.Attachments[i].Content = append([]byte(nil), copyPayload.Attachments[i].Content...)
	}
	f.payload = &copyPayload
	f.payloads = append(f.payloads, copyPayload)
	return nil
}

func (f *fakeOutboundBrokerProvider) StartChatAction(ctx context.Context, target messenger.ChatTarget, action messenger.ChatAction) (func(), error) {
	f.actions = append(f.actions, action)
	f.actionTargets = append(f.actionTargets, target)
	return func() {
		f.stoppedActions = append(f.stoppedActions, action)
	}, nil
}

func TestHandleInboundPayloadInterruptDisabledForChat(t *testing.T) {
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
	cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	chat := ensureTelegramChat(t, cfg, 42, "Test Chat", true, false)
	if err := cfg.Chat(chat.ID).SetInteractiveInterruptEnabled(false); err != nil {
		t.Fatalf("disable interrupt: %v", err)
	}
	thread := &Thread{ID: modeluuid.New(), ChatID: chat.ID, ProviderThreadID: "7", Active: true}
	sessions := &fakeBrokerSessionStore{thread: thread}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)

	result, err := broker.HandleInboundPayload(context.Background(), messenger.InboundPayload{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             messenger.TextMessage{Text: "/interrupt"},
	})
	if err != nil {
		t.Fatalf("interrupt err: %v", err)
	}
	if result.Text.Text != "interrupt is disabled for this chat" {
		t.Fatalf("interrupt payload = %#v", result)
	}
}

func TestHandleInboundPayloadRoutesGroupedContainerRefreshCommand(t *testing.T) {
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
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	ensureTelegramChat(t, cfg, 42, "Test Chat", true, false)

	broker := New(cfg, &fakeBrokerSessionStore{}, fakeBrokerSandboxManager{}, nil)
	result, err := broker.HandleInboundPayload(context.Background(), messenger.InboundPayload{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Text:             messenger.TextMessage{Text: "/container refresh"},
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if result.Text.Text != "no active conversation" {
		t.Fatalf("payload = %#v", result)
	}
}
