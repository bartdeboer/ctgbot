package appconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/go-clistate"
)

func TestNormalizeContainerPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		fallback string
		want     string
	}{
		{name: "empty uses fallback", raw: "", fallback: "/codex-home", want: "/codex-home"},
		{name: "windows absolute becomes posix", raw: `\codex-home\ctgbot-bootstrap.md`, fallback: "/codex-home", want: "/codex-home/ctgbot-bootstrap.md"},
		{name: "mixed separators cleaned", raw: `/etc\ctgbot/hostbridge-tls`, fallback: "/etc/ctgbot/hostbridge-tls", want: "/etc/ctgbot/hostbridge-tls"},
		{name: "missing leading slash", raw: `workspace`, fallback: "/workspace", want: "/workspace"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeContainerPath(tt.raw, tt.fallback); got != tt.want {
				t.Fatalf("normalizeContainerPath(%q, %q) = %q, want %q", tt.raw, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestResolveChatWorkspaceHostPathPrefersChatThenGlobalThenDefault(t *testing.T) {
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
	cfg, err := NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	globalDir := filepath.Join(root, "global-workspace")
	chatDir := filepath.Join(root, "chat-workspace")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir global workspace: %v", err)
	}
	if err := os.MkdirAll(chatDir, 0o755); err != nil {
		t.Fatalf("mkdir chat workspace: %v", err)
	}
	if err := store.PersistString("docker.workspace_host_path", globalDir); err != nil {
		t.Fatalf("persist global workspace: %v", err)
	}
	if err := cfg.PersistChatID(-123, "Test Chat"); err != nil {
		t.Fatalf("persist chat mapping: %v", err)
	}

	got, err := cfg.ResolveChatWorkspaceHostPath(-123, 0, "")
	if err != nil {
		t.Fatalf("resolve with global fallback: %v", err)
	}
	if got != globalDir {
		t.Fatalf("resolve with global fallback = %q, want %q", got, globalDir)
	}

	if err := cfg.SetChatWorkspaceHostPath(-123, chatDir); err != nil {
		t.Fatalf("set chat workspace: %v", err)
	}
	got, err = cfg.ResolveChatWorkspaceHostPath(-123, 0, "")
	if err != nil {
		t.Fatalf("resolve with chat-local workspace: %v", err)
	}
	if got != chatDir {
		t.Fatalf("resolve with chat-local workspace = %q, want %q", got, chatDir)
	}

	explicitDir := filepath.Join(root, "explicit-workspace")
	if err := os.MkdirAll(explicitDir, 0o755); err != nil {
		t.Fatalf("mkdir explicit workspace: %v", err)
	}
	got, err = cfg.ResolveChatWorkspaceHostPath(-123, 0, explicitDir)
	if err != nil {
		t.Fatalf("resolve with explicit workspace: %v", err)
	}
	if got != explicitDir {
		t.Fatalf("resolve with explicit workspace = %q, want %q", got, explicitDir)
	}
}

func TestResolveChatWorkspaceHostPathRequiresKnownChatMapping(t *testing.T) {
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
	cfg, err := NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	got, err := cfg.ResolveChatWorkspaceHostPath(-456, 2, "")
	if err == nil {
		t.Fatalf("expected unknown chat mapping error, got workspace %q", got)
	}
}

func TestEnsureChatRuntimePathsUsesChatScopedLayout(t *testing.T) {
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
	cfg, err := NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	chatID, err := modeluuid.Parse("00000000100000000000000")
	if err != nil {
		t.Fatalf("parse chat uuid: %v", err)
	}
	name, err := cfg.EnsureChatRuntimePaths(chatID)
	if err != nil {
		t.Fatalf("ensure chat runtime paths: %v", err)
	}
	if name != chatID.String() {
		t.Fatalf("runtime name = %q, want %q", name, chatID.String())
	}

	for _, dir := range []string{
		filepath.Join(root, "chats", chatID.String()),
		filepath.Join(root, "chats", chatID.String(), ".codex"),
		filepath.Join(root, "chats", chatID.String(), "workspace"),
		filepath.Join(root, "chats", chatID.String(), "logs"),
		filepath.Join(root, "chats", chatID.String(), "tls"),
		filepath.Join(root, "chats", chatID.String(), "threads"),
	} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("expected runtime dir %q to exist: %v", dir, err)
		}
	}
}

func TestChatTLSDirUsesChatScopedLayout(t *testing.T) {
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
	cfg, err := NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	chatID, err := modeluuid.Parse("00000000200000000000000")
	if err != nil {
		t.Fatalf("parse chat uuid: %v", err)
	}
	got := cfg.ChatTLSDirByID(chatID)
	want := filepath.Join(root, "chats", chatID.String(), "tls")
	if got != want {
		t.Fatalf("ChatTLSDirByID() = %q, want %q", got, want)
	}
}

func TestChatProcessToolsEnabledRoundTrip(t *testing.T) {
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
	cfg, err := NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	if err := cfg.PersistChatID(-123, "Test Chat"); err != nil {
		t.Fatalf("persist chat mapping: %v", err)
	}

	if cfg.ChatProcessToolsEnabled(-123) {
		t.Fatalf("expected process tools disabled by default")
	}
	if err := cfg.SetChatProcessToolsEnabled(-123, true); err != nil {
		t.Fatalf("set process tools enabled: %v", err)
	}
	if !cfg.ChatProcessToolsEnabled(-123) {
		t.Fatalf("expected process tools enabled")
	}
	if err := cfg.SetChatProcessToolsEnabled(-123, false); err != nil {
		t.Fatalf("set process tools disabled: %v", err)
	}
	if cfg.ChatProcessToolsEnabled(-123) {
		t.Fatalf("expected process tools disabled")
	}
}

func TestChatHostbridgeAllowedCommandsRoundTrip(t *testing.T) {
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
	cfg, err := NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("ensure paths: %v", err)
	}
	if err := cfg.PersistChatID(-123, "Test Chat"); err != nil {
		t.Fatalf("persist chat mapping: %v", err)
	}

	err = cfg.SetChatHostbridgeAllowedCommand(-123, "git-push-ctgbot", hostbridge.AllowedCommand{
		Name: "git",
		Args: []string{"push"},
		Dir:  filepath.Join(root, "ctgbot"),
	})
	if err != nil {
		t.Fatalf("set hostbridge allowed command: %v", err)
	}

	commands := cfg.ChatHostbridgeAllowedCommands(-123)
	spec, ok := commands["git-push-ctgbot"]
	if !ok {
		t.Fatalf("expected git-push-ctgbot alias")
	}
	if spec.Name != "git" {
		t.Fatalf("spec.Name = %q, want git", spec.Name)
	}
	if len(spec.Args) != 1 || spec.Args[0] != "push" {
		t.Fatalf("spec.Args = %#v, want [push]", spec.Args)
	}
	if spec.Dir != filepath.Join(root, "ctgbot") {
		t.Fatalf("spec.Dir = %q", spec.Dir)
	}
}

func TestChatHostbridgeAllowedCommandsFallsBackToLegacySpecs(t *testing.T) {
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
	cfg, err := NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	chatID := modeluuid.New()
	if err := store.PersistStruct(cfg.ChatKey(chatID, "hostbridge.allowed_commands"), []string{"/usr/bin/git"}); err != nil {
		t.Fatalf("persist legacy specs: %v", err)
	}

	commands := cfg.ChatHostbridgeAllowedCommandsByID(chatID)
	spec, ok := commands["git"]
	if !ok {
		t.Fatalf("expected legacy git alias")
	}
	if spec.Name != "/usr/bin/git" {
		t.Fatalf("spec.Name = %q, want /usr/bin/git", spec.Name)
	}
}

func TestChatContainerNameParsesUUIDs(t *testing.T) {
	root := t.TempDir()
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg, err := NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	chatID, err := modeluuid.Parse("00000000400000000000000")
	if err != nil {
		t.Fatalf("parse chat uuid: %v", err)
	}
	threadID, err := modeluuid.Parse("00000000500000000000000")
	if err != nil {
		t.Fatalf("parse thread uuid: %v", err)
	}

	name := cfg.ChatContainerName(chatID, threadID)
	gotChatID, gotThreadID, ok := cfg.ParseChatContainerName(name)
	if !ok {
		t.Fatalf("expected container name to parse")
	}
	if gotChatID != chatID || gotThreadID != threadID {
		t.Fatalf("parsed ids = (%q, %q), want (%q, %q)", gotChatID, gotThreadID, chatID, threadID)
	}
}

func TestCodexCLIHomeRootDefaultsToLocalWhenNoAuthExists(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	prevHome := os.Getenv("HOME")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", prevHome)
		_ = os.Chdir(prevWD)
	})

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg, err := NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	got := cfg.CodexCLIHomeRoot()
	want := filepath.Join(root, ".ctgbot", ".codex")
	if got != want {
		t.Fatalf("CodexCLIHomeRoot() = %q, want %q", got, want)
	}
}

func TestEnsureProviderChatPersistsExactUUIDChatKey(t *testing.T) {
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
	cfg, err := NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	entry, err := cfg.EnsureProviderChat("telegram", "-1003803364247", "Codex #2")
	if err != nil {
		t.Fatalf("EnsureProviderChat: %v", err)
	}

	chats, ok := store.Get("chats", nil).(map[string]any)
	if !ok {
		t.Fatalf("expected chats map")
	}
	if _, ok := chats[entry.ID.String()]; !ok {
		t.Fatalf("expected exact UUID chat key %q in chats map", entry.ID.String())
	}
}

func TestFindProviderChatScansChatsWithoutReverseIndex(t *testing.T) {
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
	cfg, err := NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	chatID := modeluuid.New()
	if err := store.PersistStruct("chats", map[string]any{
		chatID.String(): map[string]any{
			"chat_provider_type":  "telegram",
			"provider_chat_id":    "-1003803364247",
			"provider_chat_title": "Codex #2",
			"enabled":             true,
		},
	}); err != nil {
		t.Fatalf("persist chats map: %v", err)
	}

	entry, err := cfg.FindProviderChat("telegram", "-1003803364247")
	if err != nil {
		t.Fatalf("FindProviderChat: %v", err)
	}
	if entry == nil {
		t.Fatalf("expected migrated provider chat entry")
	}
	if entry.ID != chatID {
		t.Fatalf("resolved chat id = %q, want %q", entry.ID, chatID)
	}
}

func TestCodexCLIHomeRootPrefersExistingAuthSources(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	prevHome := os.Getenv("HOME")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", prevHome)
		_ = os.Chdir(prevWD)
	})

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg, err := NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	managedAuth := filepath.Join(home, ".ctgbot", ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(managedAuth), 0o755); err != nil {
		t.Fatalf("mkdir managed auth dir: %v", err)
	}
	if err := os.WriteFile(managedAuth, []byte("managed"), 0o600); err != nil {
		t.Fatalf("write managed auth: %v", err)
	}
	if got := cfg.CodexCLIHomeRoot(); got != filepath.Join(home, ".ctgbot", ".codex") {
		t.Fatalf("CodexCLIHomeRoot() with managed auth = %q", got)
	}

	localAuth := filepath.Join(root, ".ctgbot", ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(localAuth), 0o755); err != nil {
		t.Fatalf("mkdir local auth dir: %v", err)
	}
	if err := os.WriteFile(localAuth, []byte("local"), 0o600); err != nil {
		t.Fatalf("write local auth: %v", err)
	}
	if got := cfg.CodexCLIHomeRoot(); got != filepath.Join(root, ".ctgbot", ".codex") {
		t.Fatalf("CodexCLIHomeRoot() with local auth = %q", got)
	}
}

func TestEnsureCodexCLIHomeImportsAuthIntoSelectedLocalHome(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	prevHome := os.Getenv("HOME")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", prevHome)
		_ = os.Chdir(prevWD)
	})

	hostAuth := filepath.Join(home, ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(hostAuth), 0o755); err != nil {
		t.Fatalf("mkdir host auth dir: %v", err)
	}
	if err := os.WriteFile(hostAuth, []byte("host-auth"), 0o600); err != nil {
		t.Fatalf("write host auth: %v", err)
	}

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	localSharedHome := filepath.Join(root, ".ctgbot", ".codex")
	if err := store.PersistString("codex.cli_home_host_path", localSharedHome); err != nil {
		t.Fatalf("persist codex cli home path: %v", err)
	}
	cfg, err := NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	if err := cfg.EnsureCodexCLIHome(); err != nil {
		t.Fatalf("EnsureCodexCLIHome: %v", err)
	}

	target := filepath.Join(localSharedHome, "auth.json")
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read copied auth: %v", err)
	}
	if string(body) != "host-auth" {
		t.Fatalf("copied auth = %q, want %q", string(body), "host-auth")
	}
}
