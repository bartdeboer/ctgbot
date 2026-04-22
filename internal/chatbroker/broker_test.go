package chatbroker

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/agent"
	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/chatcommands"
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
		if err := cfg.SetChatEnabledByID(entry.ID, true); err != nil {
			t.Fatalf("set chat enabled: %v", err)
		}
	}
	if processTools {
		if err := cfg.SetChatProcessToolsEnabledByID(entry.ID, true); err != nil {
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
	if err := cfg.SetChatGPUsByID(chatID, "all"); err != nil {
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

func TestHandleIncomingMessageRoutesTelegramCommand(t *testing.T) {
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

	result, err := broker.HandleIncomingMessage(context.Background(), messenger.IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/help@ctgbot",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Text != chatcommands.New(nil).UserHelpText() {
		t.Fatalf("message text = %q, want %q", result.Messages[0].Text, chatcommands.New(nil).UserHelpText())
	}
}

func TestHandleIncomingMessageRunsUpgradeCommand(t *testing.T) {
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

	result, err := broker.HandleIncomingMessage(context.Background(), messenger.IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/upgrade",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if !process.upgradeCalled {
		t.Fatalf("expected upgrade to be called")
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Text != "upgrade completed\ntype /quit to restart" {
		t.Fatalf("message text = %q", result.Messages[0].Text)
	}
}

func TestHandleIncomingMessageRunsQuitCommand(t *testing.T) {
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

	result, err := broker.HandleIncomingMessage(context.Background(), messenger.IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/quit",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if !process.quitCalled {
		t.Fatalf("expected quit to be called")
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Text != "shutting down ctgbot" {
		t.Fatalf("message text = %q", result.Messages[0].Text)
	}
}

func TestHandleIncomingMessageStartsTypingChatAction(t *testing.T) {
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

	result, err := broker.HandleIncomingMessage(context.Background(), messenger.IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "hello",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
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

func TestHandleIncomingMessageBlocksUpgradeWithoutProcessTools(t *testing.T) {
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

	result, err := broker.HandleIncomingMessage(context.Background(), messenger.IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/upgrade",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if process.upgradeCalled {
		t.Fatalf("did not expect upgrade to be called")
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Text != "upgrade is not enabled for this chat" {
		t.Fatalf("message text = %q", result.Messages[0].Text)
	}
}

func TestHandleIncomingMessageBlocksQuitWithoutProcessTools(t *testing.T) {
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

	result, err := broker.HandleIncomingMessage(context.Background(), messenger.IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/quit",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if process.quitCalled {
		t.Fatalf("did not expect quit to be called")
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Text != "quit is not enabled for this chat" {
		t.Fatalf("message text = %q", result.Messages[0].Text)
	}
}

func TestHandleIncomingMessageRefreshesActiveConversation(t *testing.T) {
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
		RuntimeName:        cfg.ThreadContainerName(threadID),
		WorkspaceHost:      filepath.Join(root, "workspace"),
		HomeHost:           cfg.ChatCodexProfileHostPathByID(chatEntry.ID),
		ContainerWorkspace: cfg.DockerContainerWorkspacePath(),
		ContainerHome:      cfg.DockerContainerHomePath(),
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

	result, err := broker.HandleIncomingMessage(context.Background(), messenger.IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/refresh",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Text != "conversation runtime refreshed" {
		t.Fatalf("message text = %q", result.Messages[0].Text)
	}
	if sessions.thread == nil {
		t.Fatalf("expected saved thread")
	}
	if !sessions.thread.Initialized {
		t.Fatalf("expected refreshed thread to be initialized")
	}
	if sessions.thread.AgentThreadID != "agent-thread-123" {
		t.Fatalf("AgentThreadID = %q, want preserved value", sessions.thread.AgentThreadID)
	}
	if sessions.thread.LastError != "" {
		t.Fatalf("LastError = %q, want cleared", sessions.thread.LastError)
	}
}

func TestHandleIncomingMessageRefreshInstallsConfiguredSkills(t *testing.T) {
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
	if err := cfg.SetChatSkillsByID(chatEntry.ID, []string{skillTwo, skillOne}); err != nil {
		t.Fatalf("set chat skills: %v", err)
	}

	threadID := modeluuid.New()
	thread := &Thread{
		ID:                 threadID,
		ChatID:             chatEntry.ID,
		ProviderThreadID:   "7",
		AgentProviderType:  "codex",
		RuntimeName:        cfg.ThreadContainerName(threadID),
		WorkspaceHost:      filepath.Join(root, "workspace"),
		HomeHost:           cfg.ChatCodexProfileHostPathByID(chatEntry.ID),
		ContainerWorkspace: cfg.DockerContainerWorkspacePath(),
		ContainerHome:      cfg.DockerContainerHomePath(),
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

	result, err := broker.HandleIncomingMessage(context.Background(), messenger.IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/refresh",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if len(result.Messages) != 1 || result.Messages[0].Text != "conversation runtime refreshed" {
		t.Fatalf("unexpected refresh response: %#v", result.Messages)
	}
	if !reflect.DeepEqual(agent.installedSkills, []string{skillOne, skillTwo}) {
		t.Fatalf("installedSkills = %#v, want %#v", agent.installedSkills, []string{skillOne, skillTwo})
	}
}

func TestHandleIncomingMessageRefreshWithoutActiveConversation(t *testing.T) {
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

	result, err := broker.HandleIncomingMessage(context.Background(), messenger.IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/refresh",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Text != "no active conversation" {
		t.Fatalf("message text = %q", result.Messages[0].Text)
	}
}

func TestHandleIncomingMessagePurgesActiveConversation(t *testing.T) {
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
		RuntimeName:        cfg.ThreadContainerName(threadID),
		WorkspaceHost:      filepath.Join(root, "workspace"),
		HomeHost:           cfg.ChatCodexProfileHostPathByID(chatEntry.ID),
		ContainerWorkspace: cfg.DockerContainerWorkspacePath(),
		ContainerHome:      cfg.DockerContainerHomePath(),
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

	result, err := broker.HandleIncomingMessage(context.Background(), messenger.IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/purge",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Text != "conversation purged" {
		t.Fatalf("message text = %q", result.Messages[0].Text)
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

func TestHandleIncomingMessagePurgeWithoutActiveConversation(t *testing.T) {
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

	result, err := broker.HandleIncomingMessage(context.Background(), messenger.IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/purge",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Text != "no active conversation" {
		t.Fatalf("message text = %q", result.Messages[0].Text)
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

	err = broker.SendMedia(context.Background(), messenger.OutgoingMedia{
		SandboxID: thread.ID,
		Filename:  "report.pdf",
		Caption:   "Weekly report",
		Syntax:    "pdf",
		Content:   []byte("hello"),
	})
	if err != nil {
		t.Fatalf("SendMedia: %v", err)
	}
	if provider.media == nil {
		t.Fatalf("expected outbound provider to receive media")
	}
	if provider.media.ProviderChatID != "42" || provider.media.ProviderThreadID != "7" {
		t.Fatalf("unexpected provider target: %+v", *provider.media)
	}
	if provider.media.Filename != "report.pdf" || provider.media.Caption != "Weekly report" {
		t.Fatalf("unexpected outbound metadata: %+v", *provider.media)
	}
	if provider.media.Syntax != "pdf" {
		t.Fatalf("unexpected syntax: %q", provider.media.Syntax)
	}
	if string(provider.media.Content) != "hello" {
		t.Fatalf("unexpected outbound content: %q", string(provider.media.Content))
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

func (fakeBrokerAgent) HandleTurn(ctx context.Context, sbx *sandboxengine.Sandbox, providerThreadID string, prompt string) (agent.TurnResult, error) {
	return agent.TurnResult{Reply: "reply text"}, nil
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
	media          *messenger.ResolvedOutgoingMedia
	actions        []messenger.ChatAction
	actionTargets  []messenger.ChatTarget
	stoppedActions []messenger.ChatAction
}

func (f *fakeOutboundBrokerProvider) ProviderType() string { return "telegram" }

func (f *fakeOutboundBrokerProvider) SendAgentResponse(ctx context.Context, msg messenger.ResolvedOutgoingMessage) error {
	return nil
}

func (f *fakeOutboundBrokerProvider) SendMedia(ctx context.Context, media messenger.ResolvedOutgoingMedia) error {
	copyMedia := media
	copyMedia.Content = append([]byte(nil), media.Content...)
	f.media = &copyMedia
	return nil
}

func (f *fakeOutboundBrokerProvider) StartChatAction(ctx context.Context, target messenger.ChatTarget, action messenger.ChatAction) (func(), error) {
	f.actions = append(f.actions, action)
	f.actionTargets = append(f.actionTargets, target)
	return func() {
		f.stoppedActions = append(f.stoppedActions, action)
	}, nil
}

func TestHandleIncomingMessageInterruptDisabledForChat(t *testing.T) {
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
	if err := cfg.SetChatInteractiveInterruptEnabledByID(chat.ID, false); err != nil {
		t.Fatalf("disable interrupt: %v", err)
	}
	thread := &Thread{ID: modeluuid.New(), ChatID: chat.ID, ProviderThreadID: "7", Active: true}
	sessions := &fakeBrokerSessionStore{thread: thread}
	broker := New(cfg, sessions, fakeBrokerSandboxManager{}, nil)

	result, err := broker.HandleIncomingMessage(context.Background(), messenger.IncomingMessage{ProviderType: "telegram", ProviderChatID: "42", ProviderThreadID: "7", Message: "/interrupt"})
	if err != nil {
		t.Fatalf("interrupt err: %v", err)
	}
	if len(result.Messages) != 1 || result.Messages[0].Text != "interrupt is disabled for this chat" {
		t.Fatalf("interrupt messages = %#v", result.Messages)
	}
}

func TestHandleIncomingMessageRoutesGroupedContainerRefreshCommand(t *testing.T) {
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
	result, err := broker.HandleIncomingMessage(context.Background(), messenger.IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/container refresh",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	if len(result.Messages) != 1 || result.Messages[0].Text != "no active conversation" {
		t.Fatalf("messages = %#v", result.Messages)
	}
}

func TestHandleIncomingMessageDeprecatesNewCommand(t *testing.T) {
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
	result, err := broker.HandleIncomingMessage(context.Background(), messenger.IncomingMessage{
		ProviderType:     "telegram",
		ProviderChatID:   "42",
		ProviderThreadID: "7",
		Message:          "/new",
		ChatLabel:        "Test Chat",
	})
	if err != nil {
		t.Fatalf("handle incoming message: %v", err)
	}
	want := "use /container refresh to rebuild the backing container, or /chat purge to drop the active chat state"
	if len(result.Messages) != 1 || result.Messages[0].Text != want {
		t.Fatalf("messages = %#v", result.Messages)
	}
}
