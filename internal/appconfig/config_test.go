package appconfig

import (
	"os"
	"path/filepath"
	"testing"

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

func TestResolveChatWorkspaceHostPathFallsBackToManagedChatWorkspace(t *testing.T) {
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
	if err != nil {
		t.Fatalf("resolve fallback workspace: %v", err)
	}
	want := filepath.Join(root, "chats", "-456", "workspace")
	if got != want {
		t.Fatalf("resolve fallback workspace = %q, want %q", got, want)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("expected fallback workspace directory to exist: %v", err)
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

	name, err := cfg.EnsureChatRuntimePaths(-789)
	if err != nil {
		t.Fatalf("ensure chat runtime paths: %v", err)
	}
	if name != "-789" {
		t.Fatalf("runtime name = %q, want %q", name, "-789")
	}

	for _, dir := range []string{
		filepath.Join(root, "chats", "-789"),
		filepath.Join(root, "chats", "-789", ".codex"),
		filepath.Join(root, "chats", "-789", "workspace"),
		filepath.Join(root, "chats", "-789", "logs"),
		filepath.Join(root, "chats", "-789", "tls"),
		filepath.Join(root, "chats", "-789", "threads"),
	} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("expected runtime dir %q to exist: %v", dir, err)
		}
	}
}

func TestChatThreadTLSDirUsesThreadScopedLayout(t *testing.T) {
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

	got := cfg.ChatThreadTLSDir(-100, 6)
	want := filepath.Join(root, "chats", "-100", "threads", "6", "tls")
	if got != want {
		t.Fatalf("ChatThreadTLSDir() = %q, want %q", got, want)
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
