package appstate

import (
	"os"
	"path/filepath"
	"testing"

	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/go-clistate"
)

func newTestConfig(t *testing.T) (*Config, *clistate.Store) {
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
		t.Fatalf("new cwd store: %v", err)
	}
	return New(filepath.Join(root, ".ctgbot"), store), store
}

func TestDockerfileConfigDefaultsAndValidation(t *testing.T) {
	cfg, _ := newTestConfig(t)

	if got := cfg.Docker().Dockerfile(); got != "codex.Dockerfile" {
		t.Fatalf("Dockerfile() = %q, want Dockerfile", got)
	}
	if err := cfg.Docker().SetDockerfile("slim.Dockerfile"); err != nil {
		t.Fatalf("SetDockerfile() error = %v", err)
	}
	if got := cfg.Docker().Dockerfile(); got != "slim.Dockerfile" {
		t.Fatalf("Dockerfile() = %q, want slim.Dockerfile", got)
	}
	if err := cfg.Docker().SetDockerfile("../Dockerfile"); err == nil {
		t.Fatalf("expected path traversal error")
	}
}

func TestGroupedChatConfigReadsRealShapedChatConfig(t *testing.T) {
	cfg, store := newTestConfig(t)
	chatID, err := modeluuid.Parse("00VGELUQw7YRR1m4St4KGe0")
	if err != nil {
		t.Fatalf("parse chat id: %v", err)
	}

	if err := store.PersistString(cfg.Chat(chatID).key("chat_provider_type"), "telegram"); err != nil {
		t.Fatalf("persist provider type: %v", err)
	}
	if err := store.PersistBool(cfg.Chat(chatID).key("enabled"), true); err != nil {
		t.Fatalf("persist enabled: %v", err)
	}
	if err := store.PersistString(cfg.Chat(chatID).key("provider_channel_id"), "-1003759705932"); err != nil {
		t.Fatalf("persist provider channel id: %v", err)
	}
	if err := store.PersistString(cfg.Chat(chatID).key("provider_chat_title"), "Codex #1"); err != nil {
		t.Fatalf("persist title: %v", err)
	}
	if err := store.PersistString(cfg.Chat(chatID).key("container_user_mode"), "host"); err != nil {
		t.Fatalf("persist container user mode: %v", err)
	}
	if err := store.PersistString(cfg.Chat(chatID).key("workspace_host_path"), `D:\workspace`); err != nil {
		t.Fatalf("persist workspace: %v", err)
	}
	if err := store.PersistStruct(cfg.Chat(chatID).key("skills"), []string{`D:\bots\ctgbot-01\skills\human-first-coding`}); err != nil {
		t.Fatalf("persist skills: %v", err)
	}
	if err := store.PersistStruct(cfg.Chat(chatID).Hostbridge().key("allowed_commands"), map[string]hostbridgeserver.AllowedCommand{
		"git-push-workspace-docs": {
			Name:  "git",
			Args:  []string{"push"},
			Delay: "500ms",
			Dir:   `D:\workspace\WORKSPACE-DOCS`,
		},
	}); err != nil {
		t.Fatalf("persist allowed commands: %v", err)
	}

	chat := cfg.Chat(chatID)
	if got := chat.ProviderType(); got != "telegram" {
		t.Fatalf("ProviderType() = %q", got)
	}
	if !chat.Enabled() {
		t.Fatal("Enabled() = false")
	}
	if got := chat.ProviderChannelID(); got != "-1003759705932" {
		t.Fatalf("ProviderChannelID() = %q", got)
	}
	if got := chat.ProviderChatTitle(); got != "Codex #1" {
		t.Fatalf("ProviderChatTitle() = %q", got)
	}
	if got := chat.ContainerUserMode(); got != "host" {
		t.Fatalf("ContainerUserMode() = %q", got)
	}
	if got := chat.WorkspaceHostPath(); got == "" {
		t.Fatal("WorkspaceHostPath() is empty")
	}
	if got := chat.Skills(); len(got) != 1 {
		t.Fatalf("Skills() = %#v", got)
	}
	commands := chat.Hostbridge().AllowedCommands()
	if commands["git-push-workspace-docs"].Name != "git" {
		t.Fatalf("AllowedCommands() = %#v", commands)
	}
}

func TestChatHostbridgeIgnoresLegacyAllowedCommandSpecs(t *testing.T) {
	cfg, store := newTestConfig(t)
	chatID := modeluuid.New()

	if err := store.PersistStruct(cfg.Chat(chatID).Hostbridge().key("allowed_commands"), []string{"/usr/bin/git"}); err != nil {
		t.Fatalf("persist legacy allowed commands: %v", err)
	}

	if got := cfg.Chat(chatID).Hostbridge().AllowedCommands(); got != nil {
		t.Fatalf("AllowedCommands() = %#v, want nil", got)
	}
	if got := cfg.Chat(chatID).Hostbridge().ConfiguredAllowedCommands(); got != nil {
		t.Fatalf("ConfiguredAllowedCommands() = %#v, want nil", got)
	}
}

func TestChatWorkspaceFallbacksStayExplicit(t *testing.T) {
	cfg, store := newTestConfig(t)
	chatID := modeluuid.New()

	wantDefault := filepath.Join(cfg.ProjectRoot(), "chats", chatID.String(), "workspace")
	if got := cfg.Chat(chatID).WorkspaceHostPath(); got != wantDefault {
		t.Fatalf("default workspace = %q, want %q", got, wantDefault)
	}

	global := filepath.Join(t.TempDir(), "workspace")
	if err := store.PersistString("docker.workspace_host_path", global); err != nil {
		t.Fatalf("persist global workspace: %v", err)
	}
	if got := cfg.Chat(chatID).WorkspaceHostPath(); got != global {
		t.Fatalf("global workspace = %q, want %q", got, global)
	}

	chatWorkspace := filepath.Join(t.TempDir(), "chat-workspace")
	if err := store.PersistString(cfg.Chat(chatID).key("workspace_host_path"), chatWorkspace); err != nil {
		t.Fatalf("persist chat workspace: %v", err)
	}
	if got := cfg.Chat(chatID).WorkspaceHostPath(); got != chatWorkspace {
		t.Fatalf("chat workspace = %q, want %q", got, chatWorkspace)
	}
}

func TestChatCodexProfileFallbackIgnoresDiscoveredCLIHome(t *testing.T) {
	cfg, _ := newTestConfig(t)
	chatID := modeluuid.New()
	cliHome := cfg.Codex().LocalHomeRoot()
	if err := os.MkdirAll(cliHome, 0o755); err != nil {
		t.Fatalf("mkdir cli home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cliHome, "auth.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write cli auth: %v", err)
	}

	if got, want := cfg.Chat(chatID).CodexProfileHostPath(), cfg.Chat(chatID).DefaultCodexProfileDir(); got != want {
		t.Fatalf("chat codex profile = %q, want %q", got, want)
	}
}

func TestChatContainerUserModeDefaultsAndValidation(t *testing.T) {
	cfg, _ := newTestConfig(t)
	chatID := modeluuid.New()
	chat := cfg.Chat(chatID)

	if got := chat.ContainerUserMode(); got != "default" {
		t.Fatalf("ContainerUserMode() = %q, want default", got)
	}
	if err := chat.SetContainerUserMode("root"); err != nil {
		t.Fatalf("SetContainerUserMode(root) error = %v", err)
	}
	if got := chat.ContainerUserMode(); got != "root" {
		t.Fatalf("ContainerUserMode() = %q, want root", got)
	}
	if err := chat.SetContainerUserMode("danger"); err == nil {
		t.Fatalf("expected invalid mode error")
	}
}

func TestChatSettersAndKnownChats(t *testing.T) {
	cfg, _ := newTestConfig(t)
	chatID := modeluuid.New()
	chat := cfg.Chat(chatID)

	if err := chat.SetProviderType("telegram"); err != nil {
		t.Fatalf("set provider: %v", err)
	}
	if err := chat.SetProviderChannelID("123"); err != nil {
		t.Fatalf("set provider channel id: %v", err)
	}
	if err := chat.SetProviderChatTitle("Test Chat"); err != nil {
		t.Fatalf("set title: %v", err)
	}
	if err := chat.SetEnabled(true); err != nil {
		t.Fatalf("set enabled: %v", err)
	}

	found, err := cfg.FindProviderChat("telegram", "123")
	if err != nil {
		t.Fatalf("find provider channel: %v", err)
	}
	if found == nil || found.ID != chatID || !found.Enabled {
		t.Fatalf("found = %#v", found)
	}
	if chats := cfg.KnownChats(); len(chats) != 1 || chats[0].ProviderChatTitle != "Test Chat" {
		t.Fatalf("known chats = %#v", chats)
	}
}

func TestEnsureProviderChatCreatesDisabledChat(t *testing.T) {
	cfg, _ := newTestConfig(t)
	entry, err := cfg.EnsureProviderChat("telegram", "456", "Created Chat")
	if err != nil {
		t.Fatalf("ensure provider channel: %v", err)
	}
	if entry == nil || entry.ID.IsNull() {
		t.Fatalf("entry = %#v", entry)
	}
	chat := cfg.Chat(entry.ID)
	if chat.Enabled() {
		t.Fatal("new chat should default disabled")
	}
	if got := chat.ProviderChatTitle(); got != "Created Chat" {
		t.Fatalf("title = %q", got)
	}
}

func TestChatHostbridgeSetters(t *testing.T) {
	cfg, _ := newTestConfig(t)
	chatID := modeluuid.New()
	hostbridge := cfg.Chat(chatID).Hostbridge()
	if err := hostbridge.SetAllowedCommand("git-push", hostbridgeserver.AllowedCommand{Name: "git", Args: []string{"push"}}); err != nil {
		t.Fatalf("set allowed command: %v", err)
	}
	commands := hostbridge.AllowedCommands()
	if commands["git-push"].Name != "git" || len(commands["git-push"].Args) != 1 {
		t.Fatalf("commands = %#v", commands)
	}
	if err := hostbridge.RemoveAllowedCommand("git-push"); err != nil {
		t.Fatalf("remove allowed command: %v", err)
	}
	if got := hostbridge.AllowedCommands(); len(got) != 0 {
		t.Fatalf("commands after remove = %#v", got)
	}
	if err := hostbridge.ScaffoldAllowedCommand("deploy"); err != nil {
		t.Fatalf("scaffold allowed command: %v", err)
	}
	if got := hostbridge.AllowedCommands(); len(got) != 0 {
		t.Fatalf("executable commands after scaffold = %#v, want none until name is configured", got)
	}
	configured := hostbridge.ConfiguredAllowedCommands()
	if _, ok := configured["deploy"]; !ok {
		t.Fatalf("configured commands = %#v, want deploy scaffold", configured)
	}
}

func TestHostbridgeConfiguredTCPListenAddr(t *testing.T) {
	cfg, store := newTestConfig(t)

	if got := cfg.Hostbridge().ConfiguredTCPListenAddr(); got != "" {
		t.Fatalf("ConfiguredTCPListenAddr() = %q, want empty", got)
	}
	if got := cfg.Hostbridge().TCPListenAddr(); got != "127.0.0.1:4567" {
		t.Fatalf("TCPListenAddr() = %q, want default 127.0.0.1:4567", got)
	}

	if err := store.PersistString("hostbridge.tcp_listen_addr", "127.0.0.1:4568"); err != nil {
		t.Fatalf("persist listen addr: %v", err)
	}
	if got := cfg.Hostbridge().ConfiguredTCPListenAddr(); got != "127.0.0.1:4568" {
		t.Fatalf("ConfiguredTCPListenAddr() = %q, want 127.0.0.1:4568", got)
	}
	if got := cfg.Hostbridge().TCPListenAddr(); got != "127.0.0.1:4568" {
		t.Fatalf("TCPListenAddr() = %q, want 127.0.0.1:4568", got)
	}
}

func TestIdentityHelpers(t *testing.T) {
	cfg, _ := newTestConfig(t)
	chatID := modeluuid.New()
	threadID := modeluuid.New()

	if parsed, ok := cfg.ParseChatClientIdentity(cfg.Chat(chatID).ClientIdentity()); !ok || parsed != chatID {
		t.Fatalf("parse chat client identity = %s %t", parsed, ok)
	}
	if parsed, ok := cfg.ParseThreadContainerName(cfg.Thread(chatID, threadID).ContainerName()); !ok || parsed != threadID {
		t.Fatalf("parse thread container name = %s %t", parsed, ok)
	}
}

func TestChatProfileEnsurePaths(t *testing.T) {
	cfg, _ := newTestConfig(t)
	chatID := modeluuid.New()
	profile := cfg.Chat(chatID).Profile()

	if err := profile.EnsurePaths(); err != nil {
		t.Fatalf("ensure profile paths: %v", err)
	}
	for _, dir := range []string{
		profile.Root(),
		profile.CodexProfileDir(),
		profile.WorkspaceDir(),
		profile.LogDir(),
		profile.TLSDir(),
		profile.ThreadsRoot(),
	} {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			t.Fatalf("expected profile dir %q: %v", dir, err)
		}
	}
	if _, err := os.Stat(filepath.Join(profile.WorkspaceDir(), ".git")); err != nil {
		t.Fatalf("expected git workspace: %v", err)
	}
}

func TestRootSettersAndGlobalConfig(t *testing.T) {
	cfg, _ := newTestConfig(t)
	t.Setenv("HOME", t.TempDir())
	global, err := clistate.NewGlobal("ctgbot", "config")
	if err != nil {
		t.Fatalf("new global store: %v", err)
	}
	cfg.global = global

	if err := cfg.Docker().SetImage("ctgbot:test"); err != nil {
		t.Fatalf("set image: %v", err)
	}
	if got := cfg.Docker().Image(); got != "ctgbot:test" {
		t.Fatalf("image = %q", got)
	}
	if err := cfg.Codex().SetModel("gpt-test"); err != nil {
		t.Fatalf("set model: %v", err)
	}
	if got := cfg.Codex().Model(); got != "gpt-test" {
		t.Fatalf("model = %q", got)
	}
	if err := cfg.Global().SetBuildCompilerPath("/tmp/compiler"); err != nil {
		t.Fatalf("set compiler: %v", err)
	}
	if got := cfg.Global().BuildCompilerPath(); got != "/tmp/compiler" {
		t.Fatalf("compiler = %q", got)
	}
}

func TestRootProfile(t *testing.T) {
	cfg, _ := newTestConfig(t)
	profile := cfg.Profile()
	if profile.Root() != cfg.RootDir() {
		t.Fatalf("profile root = %q, want %q", profile.Root(), cfg.RootDir())
	}
	if profile.DBPath() != cfg.DBPath() {
		t.Fatalf("profile db path = %q, db path = %q", profile.DBPath(), cfg.DBPath())
	}
	if err := profile.EnsurePaths(); err != nil {
		t.Fatalf("ensure profile: %v", err)
	}
}

func TestCompatAliases(t *testing.T) {
	cfg, _ := newTestConfig(t)
	chatID := modeluuid.New()
	workspace := t.TempDir()

	if err := cfg.Chat(chatID).SetEnabled(true); err != nil {
		t.Fatalf("set enabled compat: %v", err)
	}
	if !cfg.Chat(chatID).Enabled() {
		t.Fatal("expected enabled through compat alias")
	}
	if err := cfg.Chat(chatID).SetWorkspaceHostPath(workspace); err != nil {
		t.Fatalf("set workspace compat: %v", err)
	}
	if got := cfg.Chat(chatID).WorkspaceHostPath(); got != workspace {
		t.Fatalf("workspace compat = %q, want %q", got, workspace)
	}
	if cfg.Thread(modeluuid.Nil, chatID).ContainerName() == "" {
		t.Fatal("expected thread container name")
	}
}
